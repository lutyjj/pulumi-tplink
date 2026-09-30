// Fetches the router's web-UI bundle into a local cache.
//
// Uses only node: built-ins so the whole pipeline runs inside the standard
// container image with no extra tooling (no curl, no shell). The router serves
// gzip-encoded assets from a self-signed cert, both handled here.

import * as https from "node:https";
import * as zlib from "node:zlib";
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import * as path from "node:path";

/** Hashed chunk names Vite emits, e.g. `index-Cd5ZDWXi.js`, `dhcp-CkNgD328.js`. */
const CHUNK_RE = /[A-Za-z0-9_.-]+-[A-Za-z0-9_-]{8}\.js/g;
const VERSION_RE = /<meta\s+name="version"\s+content="([^"]+)"/i;
const ENTRY_RE = /src="\.\/(js\/[A-Za-z0-9_.-]+\.js)"/i;

/** Thrown for a non-200 so the crawler can tell "absent" from "broken". */
class HttpStatusError extends Error {
    status: number | undefined;

    constructor(status: number | undefined, urlPath: string) {
        super(`GET ${urlPath} -> HTTP ${status}`);
        this.status = status;
    }
}

function get(host: string, urlPath: string): Promise<Buffer> {
    return new Promise((resolve, reject) => {
        const req = https.request(
            {
                host,
                port: 443,
                method: "GET",
                path: urlPath,
                // Stock firmware ships a self-signed certificate; scope the bypass to
                // this request rather than the process.
                rejectUnauthorized: process.env.TPLINK_INSECURE !== "true",
                headers: { "Accept-Encoding": "gzip, deflate" },
            },
            (res) => {
                if (res.statusCode !== 200) {
                    res.resume();
                    reject(new HttpStatusError(res.statusCode, urlPath));
                    return;
                }
                const parts: Buffer[] = [];
                res.on("data", (c: Buffer) => parts.push(c));
                res.on("end", () => {
                    const raw = Buffer.concat(parts);
                    const encoding = res.headers["content-encoding"];
                    // The firmware sends gzip even when it doesn't announce it, so fall
                    // back to sniffing the magic bytes.
                    const gzipped = encoding === "gzip" || (raw[0] === 0x1f && raw[1] === 0x8b);
                    if (gzipped) {
                        zlib.gunzip(raw, (err, out) => (err ? reject(err) : resolve(out)));
                    } else if (encoding === "deflate") {
                        zlib.inflate(raw, (err, out) => (err ? reject(err) : resolve(out)));
                    } else {
                        resolve(raw);
                    }
                });
            },
        );
        req.on("error", reject);
        req.setTimeout(20_000, () => req.destroy(new Error(`Timed out fetching ${urlPath}`)));
        req.end();
    });
}

export interface FetchResult {
    firmware?: string;
    chunkCount: number;
    /**
     * Chunk names referenced by the bundle that the router does not serve. The
     * hashed-name pattern also matches a few strings that are not real assets,
     * so these are expected — but they are reported rather than swallowed, since
     * a sudden jump here means the crawl heuristic has drifted.
     */
    missing: string[];
    cacheDir: string;
}

/**
 * Crawl the SPA to a fixpoint: the entry bundle only names a fraction of the
 * chunks, and lazily-imported ones reference further chunks in turn.
 */
export async function fetchBundle(host: string, cacheDir: string): Promise<FetchResult> {
    await mkdir(cacheDir, { recursive: true });

    const indexHtml = (await get(host, "/webpages/index.html")).toString("utf8");
    const firmware = VERSION_RE.exec(indexHtml)?.[1];
    const entry = ENTRY_RE.exec(indexHtml)?.[1];
    if (!entry) throw new Error("Could not find the entry bundle in index.html");

    const seen = new Set<string>();
    const missing: string[] = [];
    const queue = [path.basename(entry)];

    while (queue.length > 0) {
        const name = queue.shift() as string;
        if (seen.has(name)) continue;
        seen.add(name);

        let body: string;
        try {
            body = (await get(host, `/webpages/js/${name}`)).toString("utf8");
        } catch (e) {
            // Referenced-but-absent chunks are skipped; anything else is a real fault.
            if (e instanceof HttpStatusError) {
                missing.push(name);
                continue;
            }
            throw e;
        }
        await writeFile(path.join(cacheDir, name), body, "utf8");

        for (const ref of body.matchAll(CHUNK_RE)) {
            if (!seen.has(ref[0])) queue.push(ref[0]);
        }
    }

    if (firmware) await writeFile(path.join(cacheDir, "firmware.txt"), firmware, "utf8");
    return { firmware, chunkCount: seen.size - missing.length, missing, cacheDir };
}

/** Read a previously fetched bundle back out of the cache. */
export async function readCache(
    cacheDir: string,
): Promise<{ firmware?: string; chunks: { name: string; source: string }[] }> {
    const entries = await readdir(cacheDir);
    const chunks = await Promise.all(
        entries
            .filter((f) => f.endsWith(".js"))
            .sort()
            .map(async (name) => ({
                name,
                source: await readFile(path.join(cacheDir, name), "utf8"),
            })),
    );
    const firmware = entries.includes("firmware.txt")
        ? (await readFile(path.join(cacheDir, "firmware.txt"), "utf8")).trim()
        : undefined;
    return { firmware, chunks };
}
