// AST-based extraction of the router's HTTP API surface from its shipped web-UI bundle.
//
// The UI is a code-split Vue SPA whose chunks are minified but not obfuscated:
// string literals and object property names survive, which is enough to recover
// the whole endpoint/operation surface. Everything here is derived — no endpoint
// list is hardcoded.
//
// Parsing goes through the TypeScript compiler's JS parser rather than regexes:
// minified code puts many declarations on one line (`const a="x",b="y"`) and
// nests calls arbitrarily, which regexes get wrong in both directions.

import ts from "typescript";
import type { Endpoint, EndpointOperation, WireShape } from "./ir.mts";

/** `admin/dhcps?form=reservation`, `login?form=keys` — optionally slash-prefixed. */
const ENDPOINT_RE = /^\/?(?:([a-z_][a-z_0-9]*)\/)?([a-z_][a-z_0-9]*)\?form=([a-z_0-9]+)$/;

/**
 * The generic request adapter's verbs. Each maps to a fixed `operation=` value
 * except `request`, which is the escape hatch carrying an explicit operation.
 */
const ADAPTER_VERBS = new Set(["read", "write", "load", "insert", "remove", "update", "file"]);
const ESCAPE_HATCH = "request";

interface ParsedEndpoint {
    path: string;
    namespace: string;
    form: string;
}

function parseEndpoint(raw: string): ParsedEndpoint | undefined {
    const m = ENDPOINT_RE.exec(raw);
    if (!m) return undefined;
    const [, prefix, tail, form] = m;
    // `/admin/dhcps?form=x` -> path `admin/dhcps`, namespace `dhcps`.
    // `login?form=x`       -> path `login`,       namespace `login`.
    return { path: prefix ? `${prefix}/${tail}` : tail, namespace: tail, form };
}

function endpointKey(e: ParsedEndpoint): string {
    return `${e.path}?form=${e.form}`;
}

/** Literal string value of a node, if it is one. */
function literal(node: ts.Node | undefined): string | undefined {
    return node && ts.isStringLiteralLike(node) ? node.text : undefined;
}

/** Property names of an object literal (shorthand and assignment alike). */
function propertyNames(obj: ts.ObjectLiteralExpression): string[] {
    const names: string[] = [];
    for (const p of obj.properties) {
        if (ts.isShorthandPropertyAssignment(p)) names.push(p.name.text);
        else if (
            ts.isPropertyAssignment(p) &&
            (ts.isIdentifier(p.name) || ts.isStringLiteralLike(p.name))
        ) {
            names.push(p.name.text);
        }
    }
    return names;
}

/** Value of a named property in an object literal, if it is a string literal. */
function literalProperty(obj: ts.ObjectLiteralExpression, key: string): string | undefined {
    for (const p of obj.properties) {
        if (!ts.isPropertyAssignment(p)) continue;
        const name =
            ts.isIdentifier(p.name) || ts.isStringLiteralLike(p.name) ? p.name.text : undefined;
        if (name === key) return literal(p.initializer);
    }
    return undefined;
}

function walk(node: ts.Node, visit: (n: ts.Node) => void): void {
    visit(node);
    node.forEachChild((c) => walk(c, visit));
}

/**
 * Pass 1 — every `<identifier> = "<endpoint>"` binding in the chunk.
 *
 * Collected before call sites because minified modules routinely reference a
 * binding declared later in the file.
 */
function collectBindings(source: ts.SourceFile): Map<string, ParsedEndpoint> {
    const bindings = new Map<string, ParsedEndpoint>();
    walk(source, (n) => {
        if (!ts.isVariableDeclaration(n) || !n.initializer || !ts.isIdentifier(n.name)) return;
        const raw = literal(n.initializer);
        const ep = raw && parseEndpoint(raw);
        if (ep) bindings.set(n.name.text, ep);
    });
    return bindings;
}

/**
 * Pass 2 — mapper functions of the form
 *   `function u(t){ const {key,enable,hostname,mac,ip} = t; return {enable:…,key,hostname,mac,ip} }`
 *
 * These are how the UI converts a view model to the wire payload, so the
 * returned object's property names ARE the entity's wire fields. Requiring an
 * overlap with a destructuring pattern in the same function keeps incidental
 * object literals (style objects, Vue props) out.
 */
function collectShapes(source: ts.SourceFile, chunk: string): WireShape[] {
    const shapes: WireShape[] = [];

    const inspect = (body: ts.Node): void => {
        const destructured = new Set<string>();
        walk(body, (n) => {
            if (ts.isVariableDeclaration(n) && ts.isObjectBindingPattern(n.name)) {
                for (const el of n.name.elements) {
                    if (ts.isIdentifier(el.name))
                        destructured.add((el.propertyName ?? el.name).getText());
                }
            }
        });
        if (destructured.size < 2) return;

        walk(body, (n) => {
            if (
                !ts.isReturnStatement(n) ||
                !n.expression ||
                !ts.isObjectLiteralExpression(n.expression)
            )
                return;
            const fields = propertyNames(n.expression);
            if (fields.length < 2) return;
            const overlap = fields.filter((f) => destructured.has(f)).length;
            // Majority of the returned shape must come from the destructured input.
            if (overlap < 2 || overlap * 2 < fields.length) return;
            shapes.push({ fields: [...new Set(fields)].sort(), chunk });
        });
    };

    walk(source, (n) => {
        if (ts.isFunctionDeclaration(n) || ts.isFunctionExpression(n) || ts.isArrowFunction(n)) {
            if (n.body) inspect(n.body);
        }
    });
    return shapes;
}

/**
 * Pass 3 — adapter call sites: `<store>.<verb>(<endpoint>, …)`.
 *
 * The first argument identifies the endpoint (literal or binding). For the
 * `request` escape hatch the operation is read from the options object instead
 * of the verb, and its sibling keys are recorded as request fields.
 */
function collectCalls(
    source: ts.SourceFile,
    bindings: Map<string, ParsedEndpoint>,
    sink: (ep: ParsedEndpoint, op: string, fields: string[]) => void,
): void {
    walk(source, (n) => {
        if (!ts.isCallExpression(n) || !ts.isPropertyAccessExpression(n.expression)) return;
        const verb = n.expression.name.text;
        if (!ADAPTER_VERBS.has(verb) && verb !== ESCAPE_HATCH) return;

        const arg0 = n.arguments[0];
        if (!arg0) return;
        const raw = literal(arg0);
        const ep = raw
            ? parseEndpoint(raw)
            : ts.isIdentifier(arg0)
              ? bindings.get(arg0.text)
              : undefined;
        if (!ep) return;

        const arg1 = n.arguments[1];
        if (verb === ESCAPE_HATCH) {
            if (!arg1 || !ts.isObjectLiteralExpression(arg1)) return;
            const op = literalProperty(arg1, "operation");
            if (!op) return;
            sink(
                ep,
                op,
                propertyNames(arg1).filter((f) => f !== "operation"),
            );
            return;
        }

        const fields = arg1 && ts.isObjectLiteralExpression(arg1) ? propertyNames(arg1) : [];
        sink(ep, verb, fields);
    });
}

export interface ExtractInput {
    /** Chunk file name (kept for provenance in the IR). */
    name: string;
    source: string;
}

/** Recover the endpoint surface from a set of web-UI chunks. */
export function extract(chunks: ExtractInput[]): { endpoints: Endpoint[]; shapes: WireShape[] } {
    const byEndpoint = new Map<string, Endpoint>();
    const shapes: WireShape[] = [];

    const upsert = (parsed: ParsedEndpoint): Endpoint => {
        const key = endpointKey(parsed);
        let ep = byEndpoint.get(key);
        if (!ep) {
            ep = {
                path: parsed.path,
                namespace: parsed.namespace,
                form: parsed.form,
                operations: [],
                chunks: [],
            };
            byEndpoint.set(key, ep);
        }
        return ep;
    };

    for (const chunk of chunks) {
        const source = ts.createSourceFile(
            chunk.name,
            chunk.source,
            ts.ScriptTarget.ESNext,
            true,
            ts.ScriptKind.JS,
        );
        const bindings = collectBindings(source);

        // An endpoint literal anywhere in a chunk is provenance even if its call
        // site lives in another chunk (re-exported helpers).
        walk(source, (n) => {
            const raw = literal(n);
            const parsed = raw ? parseEndpoint(raw) : undefined;
            if (!parsed) return;
            const ep = upsert(parsed);
            if (!ep.chunks.includes(chunk.name)) ep.chunks.push(chunk.name);
        });

        collectCalls(source, bindings, (parsed, operation, fields) => {
            const ep = upsert(parsed);
            let op: EndpointOperation | undefined = ep.operations.find(
                (o) => o.operation === operation,
            );
            if (!op) {
                op = { operation, requestFields: [] };
                ep.operations.push(op);
            }
            for (const f of fields) if (!op.requestFields.includes(f)) op.requestFields.push(f);
        });

        shapes.push(...collectShapes(source, chunk.name));
    }

    const endpoints = [...byEndpoint.values()].sort((a, b) =>
        `${a.path}${a.form}`.localeCompare(`${b.path}${b.form}`),
    );
    for (const ep of endpoints) {
        ep.operations.sort((a, b) => a.operation.localeCompare(b.operation));
        for (const op of ep.operations) op.requestFields.sort();
        ep.chunks.sort();
    }
    return { endpoints, shapes };
}
