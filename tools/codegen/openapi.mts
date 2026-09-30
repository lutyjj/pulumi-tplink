// Renders the extracted IR as an OpenAPI 3.1 document.
//
// Modelling note. Every call is `POST <prefix>/<path>?form=<form>` with an
// `operation=` discriminator in a form-encoded body, so the natural unit is the
// (path, form, operation) triple — but OpenAPI keys operations by path + method
// and cannot express "same path and method, different query string". Encoding
// the triple as a synthetic path keeps one operationId per real call, which is
// what makes the generated client usable; the true request URL travels in the
// `x-tplink` extension so an emitter can rebuild it exactly. `emit-ts.mts` does
// this; a stock third-party generator would need the same one-line hook.

import type { ExtractedApi } from "./ir.mts";
import { type InferredSchema, topLevelFields } from "./infer.mts";

/** Response schemas from the sampling pass, keyed `<path>?form=<form>#<operation>`. */
export type SampledSchemas = Record<string, InferredSchema>;

/** Hand-maintained additions merged over the derived document. */
export interface Overlay {
    /** Response `data` schemas, keyed by `<path>?form=<form>#<operation>`. */
    responses?: Record<string, unknown>;
    /** Extra/corrected request fields, same key format. */
    requestFields?: Record<string, string[]>;
    /** Free-text notes surfaced as operation descriptions. */
    descriptions?: Record<string, string>;
}

export interface OpenApiDocument {
    openapi: string;
    info: Record<string, unknown>;
    servers: unknown[];
    paths: Record<string, unknown>;
    components: Record<string, unknown>;
}

const SCHEMA_STRING = { type: "string" } as const;

/** `x-tplink.responseSchemaFrom` marker for a schema observed by the sampling pass. */
const SAMPLED = "sampled";

/**
 * How many operations in an already-rendered spec carry a sampled response
 * schema — i.e. how much a rebuild without the sampling pass would discard.
 */
export function sampledOperations(spec: unknown): number {
    const paths = (spec as { paths?: Record<string, { post?: { "x-tplink"?: unknown } }> })?.paths;
    return Object.values(paths ?? {}).filter(
        (p) =>
            (p.post?.["x-tplink"] as { responseSchemaFrom?: string } | undefined)
                ?.responseSchemaFrom === SAMPLED,
    ).length;
}

/** `admin/dhcps` + `reservation` + `insert` -> `dhcpsReservationInsert`. */
function operationId(namespace: string, form: string, operation: string): string {
    const parts = [namespace, form, operation]
        .flatMap((p) => p.split(/[^A-Za-z0-9]+/))
        .filter(Boolean);
    return parts
        .map((p, i) => {
            // Preserve intra-word capitals (`loadDevice`) but normalise the boundary.
            const head = i === 0 ? p[0].toLowerCase() : p[0].toUpperCase();
            return head + p.slice(1);
        })
        .join("");
}

function overlayKey(path: string, form: string, operation: string): string {
    return `${path}?form=${form}#${operation}`;
}

export function buildSpec(
    api: ExtractedApi,
    overlay: Overlay,
    sampled: SampledSchemas = {},
): OpenApiDocument {
    const paths: Record<string, unknown> = {};

    for (const endpoint of api.endpoints) {
        for (const op of endpoint.operations) {
            const key = overlayKey(endpoint.path, endpoint.form, op.operation);
            const extraFields = overlay.requestFields?.[key] ?? [];

            // By LuCI convention a `write` accepts the same fields its sibling
            // `read` returns, and the UI builds those bodies from a variable that
            // static analysis cannot see into. When the read side has been sampled,
            // borrow its field names — flagged below, since this is a convention
            // rather than an observation.
            const pairedReadKey = overlayKey(endpoint.path, endpoint.form, "read");
            const inheritedFromRead =
                op.operation === "write" &&
                op.requestFields.length === 0 &&
                extraFields.length === 0
                    ? topLevelFields(sampled[pairedReadKey])
                    : [];

            const fields = [
                ...new Set([...op.requestFields, ...extraFields, ...inheritedFromRead]),
            ].sort();

            // Form-encoded bodies carry no types: every field is a string on the wire.
            const properties: Record<string, unknown> = {
                operation: { type: "string", const: op.operation },
            };
            for (const f of fields) properties[f] = SCHEMA_STRING;

            // Precedence: hand-pinned overlay > observed sample > unknown.
            const dataSchema = overlay.responses?.[key] ?? sampled[key] ?? {};
            const provenance = overlay.responses?.[key]
                ? "overlay"
                : sampled[key]
                  ? SAMPLED
                  : "none";
            const synthetic = `/${endpoint.path}/${endpoint.form}/${op.operation}`;

            paths[synthetic] = {
                post: {
                    operationId: operationId(endpoint.namespace, endpoint.form, op.operation),
                    summary: `${op.operation} ${endpoint.path}?form=${endpoint.form}`,
                    ...(overlay.descriptions?.[key]
                        ? { description: overlay.descriptions[key] }
                        : {}),
                    tags: [endpoint.namespace],
                    "x-tplink": {
                        path: endpoint.path,
                        form: endpoint.form,
                        operation: op.operation,
                        // Real request target, relative to the server URL.
                        urlTemplate: `/;stok={stok}/${endpoint.path}?form=${endpoint.form}`,
                        // Where the schemas came from, so a consumer can judge them.
                        responseSchemaFrom: provenance,
                        ...(inheritedFromRead.length > 0
                            ? { requestFieldsFrom: "paired-read" }
                            : {}),
                        // Provenance, so a finding can be re-checked against the bundle.
                        chunks: endpoint.chunks,
                    },
                    requestBody: {
                        required: true,
                        content: {
                            "application/x-www-form-urlencoded": {
                                schema: { type: "object", required: ["operation"], properties },
                            },
                        },
                    },
                    responses: {
                        "200": {
                            description: "Router response envelope",
                            content: {
                                "application/json": {
                                    schema: {
                                        allOf: [
                                            { $ref: "#/components/schemas/Envelope" },
                                            { type: "object", properties: { data: dataSchema } },
                                        ],
                                    },
                                },
                            },
                        },
                    },
                },
            };
        }
    }

    return {
        openapi: "3.1.0",
        info: {
            title: "TP-Link router web API",
            version: api.firmware ?? "unknown",
            description:
                "Reverse-engineered from the router's shipped web-UI bundle by tools/tplink-codegen. " +
                "Generated file — do not edit by hand; run `make codegen`. " +
                "Request field lists are those observed at UI call sites and may be incomplete; " +
                "response `data` schemas are empty unless pinned in overlay.json.",
        },
        servers: [
            {
                url: "https://{host}/cgi-bin/luci",
                variables: { host: { default: "192.168.0.1", description: "Router LAN address" } },
            },
        ],
        paths,
        components: {
            schemas: {
                Envelope: {
                    type: "object",
                    required: ["success"],
                    properties: {
                        success: { type: "boolean" },
                        errorcode: { type: "string" },
                        data: {},
                    },
                },
            },
        },
    };
}
