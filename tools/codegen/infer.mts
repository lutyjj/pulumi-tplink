// Infers JSON Schema from observed response payloads.
//
// Only ever handed values in memory: the caller must not persist raw samples,
// which contain live secrets (WiFi PSKs, account details). Schemas carry field
// names and types only.

/** Structural JSON Schema subset — matches what emit-ts can render. */
export interface InferredSchema {
    type?: "string" | "number" | "boolean" | "object" | "array";
    properties?: Record<string, InferredSchema>;
    items?: InferredSchema;
}

/** Empty schema — "something was here, but its shape is unknown". */
const UNKNOWN: InferredSchema = {};

function isUnknown(s: InferredSchema): boolean {
    return Object.keys(s).length === 0;
}

/** Schema describing a single observed value. */
export function inferOne(value: unknown): InferredSchema {
    if (value === null || value === undefined) return UNKNOWN;
    if (typeof value === "string") return { type: "string" };
    if (typeof value === "number") return { type: "number" };
    if (typeof value === "boolean") return { type: "boolean" };
    if (Array.isArray(value)) {
        // An empty array tells us nothing about its element type.
        const items = value.map(inferOne).reduce<InferredSchema>(merge, UNKNOWN);
        return isUnknown(items) ? { type: "array" } : { type: "array", items };
    }
    if (typeof value === "object") {
        const properties: Record<string, InferredSchema> = {};
        for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
            properties[k] = inferOne(v);
        }
        return { type: "object", properties };
    }
    return UNKNOWN;
}

/**
 * Combines two schemas describing the same position.
 *
 * A field absent from one sample and present in another is kept (everything is
 * optional anyway); genuinely conflicting types collapse to unknown rather than
 * inventing a union the wire format cannot express.
 */
export function merge(a: InferredSchema, b: InferredSchema): InferredSchema {
    if (isUnknown(a)) return b;
    if (isUnknown(b)) return a;
    if (a.type !== b.type) return UNKNOWN;

    if (a.type === "object") {
        const properties: Record<string, InferredSchema> = { ...a.properties };
        for (const [k, v] of Object.entries(b.properties ?? {})) {
            properties[k] = k in properties ? merge(properties[k], v) : v;
        }
        return { type: "object", properties };
    }
    if (a.type === "array") {
        const items = a.items && b.items ? merge(a.items, b.items) : (a.items ?? b.items);
        return items ? { type: "array", items } : { type: "array" };
    }
    return a;
}

/**
 * Top-level field names of a response, when it is an object. This is what makes
 * a paired `write` typeable: by LuCI convention a `write` accepts the same
 * fields its sibling `read` returns.
 */
export function topLevelFields(schema: InferredSchema | undefined): string[] {
    if (schema?.type !== "object") return [];
    return Object.keys(schema.properties ?? {}).sort();
}
