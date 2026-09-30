// CLI for the router API codegen pipeline. Runs inside the repo's node container
// (see `make api-spec`) — it needs no tooling beyond the
// image and imports nothing outside `tools/codegen/`.
//
//   fetch     crawl the router's web UI into .cache/
//   extract   parse the cache into an endpoint IR
//   sample    call every read-only endpoint once and infer response schemas
//   spec      render the IR (plus overlay) as an OpenAPI 3.1 document
//   all       fetch -> extract -> spec

import { mkdir, readFile, writeFile } from "node:fs/promises";
import * as path from "node:path";
import { fetchBundle, readCache } from "./fetch.mts";
import { extract } from "./extract.mts";
import type { ExtractedApi } from "./ir.mts";
import { buildSpec, sampledOperations, type SampledSchemas } from "./openapi.mts";
import { sampleResponses } from "./sample.mts";

const HERE = import.meta.dirname;
const CACHE = path.join(HERE, ".cache");
const IR_PATH = path.join(CACHE, "api.json");
// Only inferred schemas are persisted — never the sampled payloads, which carry
// live credentials (a wireless read returns the PSK).
const SCHEMAS_PATH = path.join(CACHE, "response-schemas.json");
const OVERLAY_PATH = path.join(HERE, "overlay.json");
const SPEC_PATH = path.join(HERE, "..", "..", "api", "openapi.json");

async function routerHost(argv: string[]): Promise<string> {
 const flag = argv.indexOf("--host");
 const host = flag !== -1 ? argv[flag + 1] : process.env.TPLINK_HOST;
 if (!host) throw new Error("Pass --host or set TPLINK_HOST.");
 return host;
}

async function readJson<T>(file: string, fallback: T): Promise<T> {
    try {
        return JSON.parse(await readFile(file, "utf8")) as T;
    } catch {
        return fallback;
    }
}

async function doFetch(argv: string[]): Promise<void> {
    const host = await routerHost(argv);
    console.log(`fetching web UI from ${host} …`);
    const { firmware, chunkCount } = await fetchBundle(host, CACHE);
    console.log(`  firmware: ${firmware ?? "(unknown)"}`);
    console.log(`  chunks:   ${chunkCount}`);
}

async function doExtract(): Promise<ExtractedApi> {
    const { firmware, chunks } = await readCache(CACHE);
    if (chunks.length === 0) throw new Error(`No cached bundle in ${CACHE} — run \`fetch\` first.`);
    const { endpoints, shapes } = extract(chunks);
    const api: ExtractedApi = { firmware, endpoints, shapes };

    await mkdir(CACHE, { recursive: true });
    await writeFile(IR_PATH, `${JSON.stringify(api, null, 2)}\n`, "utf8");

    const ops = endpoints.reduce((n, e) => n + e.operations.length, 0);
    const resolved = endpoints.filter((e) => e.operations.length > 0).length;
    console.log(`extracted from ${chunks.length} chunks:`);
    console.log(`  endpoints:  ${endpoints.length} (${resolved} with resolved operations)`);
    console.log(`  operations: ${ops}`);
    console.log(`  wire shapes: ${shapes.length}`);
    return api;
}

async function doSample(argv: string[]): Promise<void> {
    const host = await routerHost(argv);
    const password = process.env.TPLINK_PASSWORD;
    if (!password) {
        throw new Error("TPLINK_PASSWORD is not set — run via `make api-spec-sampled`.");
    }
    const api = await readJson<ExtractedApi | undefined>(IR_PATH, undefined);
    if (!api) throw new Error(`No IR at ${IR_PATH} — run \`extract\` first.`);

    console.log(`sampling read-only endpoints on ${host} …`);
    const report = await sampleResponses(host, password, api.endpoints);
    await writeFile(SCHEMAS_PATH, `${JSON.stringify(report.schemas, null, 2)}\n`, "utf8");
    console.log(
        `  attempted: ${report.attempted}  ok: ${report.succeeded}  skipped (unsafe): ${report.skipped}`,
    );
    if (report.failures.length > 0) {
        console.log(`  failed:    ${report.failures.length}`);
        for (const f of report.failures.slice(0, 10)) console.log(`    - ${f}`);
        if (report.failures.length > 10) console.log(`    … ${report.failures.length - 10} more`);
    }
}

async function doSpec(): Promise<void> {
    const api = await readJson<ExtractedApi | undefined>(IR_PATH, undefined);
    if (!api) throw new Error(`No IR at ${IR_PATH} — run \`extract\` first.`);
    const overlay = await readJson<Record<string, unknown>>(OVERLAY_PATH, {});
    const sampled = await readJson<SampledSchemas>(SCHEMAS_PATH, {});
    // The schema cache is gitignored, so a fresh clone or worktree has none. Left
    // to itself the pipeline would then quietly rewrite the committed spec with
    // static analysis only, deleting every sampled response schema in it. Refuse
    // instead — but only when there is something to lose, so a first-ever static
    // build still works.
    if (Object.keys(sampled).length === 0) {
        const committed = sampledOperations(await readJson<unknown>(SPEC_PATH, undefined));
        if (committed > 0) {
            throw new Error(
                `No sampled response schemas in ${path.relative(process.cwd(), SCHEMAS_PATH)}, but ` +
                    `${path.relative(process.cwd(), SPEC_PATH)} has ${committed} sampled operations that this run would drop. ` +
                    "Run `make api-spec-sampled` (needs TPLINK_PASSWORD) instead of `make api-spec`.",
            );
        }
    }
    const spec = buildSpec(api, overlay, sampled);
    await writeFile(SPEC_PATH, `${JSON.stringify(spec, null, 2)}\n`, "utf8");
    console.log(
        `wrote ${path.relative(process.cwd(), SPEC_PATH)} (${Object.keys(spec.paths).length} paths)`,
    );
}

const argv = process.argv.slice(2);
const command = argv[0] ?? "all";

switch (command) {
    case "fetch":
        await doFetch(argv);
        break;
    case "extract":
        await doExtract();
        break;
    case "sample":
        await doSample(argv);
        break;
    case "spec":
        await doSpec();
        break;
    case "all":
        await doFetch(argv);
        await doExtract();
        if (process.env.TPLINK_PASSWORD) await doSample(argv);
        await doSpec();
        break;
    default:
        console.error(
            `unknown command: ${command}\nusage: main.mts [fetch|extract|sample|spec|all] [--host <ip>]`,
        );
        process.exit(2);
}
