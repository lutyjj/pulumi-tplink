// Read-only sampling pass: calls every safe endpoint once and infers its
// response schema.
//
// Static analysis recovers *which* endpoints exist but not what they return, nor
// what the `write` half of a read/write pair accepts (the UI builds those bodies
// from a variable). One authenticated sweep of the read side supplies both.
//
// Safety. Only `read` and `load` are ever issued — the two verbs that cannot
// mutate — and a namespace denylist excludes anything whose *read* could still
// disrupt the device or the session. Nothing else is called, so no `write`,
// `set`, `upgrade`, `factory` or `reboot` can be reached from here.
//
// Secrets. Responses contain live credentials (a wireless read returns the PSK).
// Payloads are inferred in memory and discarded; only field names and types
// leave this module.

import * as https from "node:https";
import * as crypto from "node:crypto";
import type { Endpoint } from "./ir.mts";
import { type InferredSchema, inferOne, merge } from "./infer.mts";

/** Verbs that are safe to issue. Everything else is out of scope by construction. */
const SAFE_OPERATIONS = new Set(["read", "load"]);

/**
 * Namespaces skipped even for read/load.
 *
 * `login` would disturb the session we are borrowing; the rest are firmware,
 * recovery and diagnostic surfaces where a read can kick off a background action
 * (upgrade checks, network scans, log dumps) that has no business running as a
 * side effect of code generation.
 */
const DENIED_NAMESPACES = new Set([
    "login",
    "firmware",
    "reboot",
    "upgrade",
    "debug",
    "diag",
    "device_config",
    "avira_network_check",
    "cloud_account",
    "blocking",
    "syslog",
]);

const LUCI = "/cgi-bin/luci/;stok=";

interface Session {
    host: string;
    stok: string;
    cookie: string;
}

/** Router response envelope, before any endpoint-specific interpretation. */
interface RawResponse {
    success?: boolean;
    errorcode?: string;
    data?: unknown;
}

function rawPost(
    host: string,
    path: string,
    body: string,
    cookie?: string,
): Promise<{ json: RawResponse; cookie?: string }> {
    return new Promise((resolve, reject) => {
        const req = https.request(
            {
                host,
                port: 443,
                method: "POST",
                path,
                rejectUnauthorized: process.env.TPLINK_INSECURE !== "true",
                headers: {
                    "Content-Type": "application/x-www-form-urlencoded",
                    "Content-Length": Buffer.byteLength(body),
                    Referer: `https://${host}/webpages/index.html`,
                    Origin: `https://${host}`,
                    ...(cookie ? { Cookie: cookie } : {}),
                },
            },
            (res) => {
                let data = "";
                res.on("data", (c) => {
                    data += c;
                });
                res.on("end", () => {
                    const setCookie = res.headers["set-cookie"]?.[0]?.split(";")[0];
                    try {
                        resolve({ json: JSON.parse(data), cookie: setCookie });
                    } catch {
                        reject(new Error(`Non-JSON response from ${path}`));
                    }
                });
            },
        );
        req.on("error", reject);
        req.setTimeout(20_000, () => req.destroy(new Error(`Timed out on ${path}`)));
        req.write(body);
        req.end();
    });
}

/**
 * RSA-only login. Deliberately re-implemented rather than imported from the
 * provider: this directory stays free of repo-internal imports so it can be
 * lifted out wholesale.
 */
async function login(host: string, password: string): Promise<Session> {
    const keys = await rawPost(host, `${LUCI}/login?form=keys`, "operation=read");
    const pubkey = (keys.json.data as { password?: [string, string] } | undefined)?.password;
    if (!pubkey) throw new Error("Router did not return a login key");
    const [nHex, eHex] = pubkey;
    const pub = crypto.createPublicKey({
        key: {
            kty: "RSA",
            n: Buffer.from(nHex, "hex").toString("base64url"),
            e: Buffer.from(eHex.length % 2 ? `0${eHex}` : eHex, "hex").toString("base64url"),
        },
        format: "jwk",
    });
    const encrypted = crypto
        .publicEncrypt(
            { key: pub, padding: crypto.constants.RSA_PKCS1_PADDING },
            Buffer.from(password, "utf8"),
        )
        .toString("hex");
    const res = await rawPost(
        host,
        `${LUCI}/login?form=login`,
        `operation=login&password=${encrypted}`,
    );
    if (!res.json.success)
        throw new Error(`Router login failed: ${res.json.errorcode ?? "unknown"}`);
    const { stok } = res.json.data as { stok: string };
    return { host, stok, cookie: res.cookie ?? "" };
}

/** Response schemas keyed by `<path>?form=<form>#<operation>`. */
export type ResponseSchemas = Record<string, InferredSchema>;

export interface SampleReport {
    schemas: ResponseSchemas;
    attempted: number;
    succeeded: number;
    skipped: number;
    failures: string[];
}

export function isSampleable(endpoint: Endpoint, operation: string): boolean {
    return SAFE_OPERATIONS.has(operation) && !DENIED_NAMESPACES.has(endpoint.namespace);
}

/**
 * Issue one read/load per safe endpoint and infer response schemas.
 *
 * The router permits a single admin session, so this logs in once, walks the
 * endpoints sequentially, and logs out — never in parallel with a deploy.
 */
export async function sampleResponses(
    host: string,
    password: string,
    endpoints: Endpoint[],
): Promise<SampleReport> {
    const session = await login(host, password);
    const schemas: ResponseSchemas = {};
    const failures: string[] = [];
    let attempted = 0;
    let succeeded = 0;
    let skipped = 0;

    try {
        for (const endpoint of endpoints) {
            for (const op of endpoint.operations) {
                const key = `${endpoint.path}?form=${endpoint.form}#${op.operation}`;
                if (!isSampleable(endpoint, op.operation)) {
                    skipped++;
                    continue;
                }
                attempted++;
                try {
                    const res = await rawPost(
                        host,
                        `${LUCI}${session.stok}/${endpoint.path}?form=${endpoint.form}`,
                        `operation=${op.operation}`,
                        session.cookie,
                    );
                    if (!res.json.success) {
                        failures.push(`${key}: ${res.json.errorcode ?? "not successful"}`);
                        continue;
                    }
                    // Infer immediately; the payload is never stored or logged.
                    const inferred = inferOne(res.json.data);
                    schemas[key] = key in schemas ? merge(schemas[key], inferred) : inferred;
                    succeeded++;
                } catch (e) {
                    failures.push(`${key}: ${(e as Error).message}`);
                }
            }
        }
    } finally {
        await rawPost(
            host,
            `${LUCI}${session.stok}/admin/system?form=logout`,
            "operation=write",
            session.cookie,
        ).catch(() => {});
    }

    return { schemas, attempted, succeeded, skipped, failures };
}
