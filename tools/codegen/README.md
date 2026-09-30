# API recovery

This development tool derives the router's HTTP endpoint surface from its shipped
web bundles. It writes `api/openapi.json`, not the Pulumi resource schema. The Go
provider calls only its managed endpoints; there is no generated all-endpoint client.

```text
fetch -> .cache/*.js -> extract -> .cache/api.json -> spec -> api/openapi.json
                                                     ^
                                  overlay.json + sampled schema shapes
```

Run through the containerised Makefile:

```sh
TPLINK_HOST=<router-address> TPLINK_INSECURE=true make api-spec
TPLINK_HOST=<router-address> TPLINK_INSECURE=true \
  TPLINK_PASSWORD=<admin-password> make api-spec-sampled
```

Supply the password through a protected environment source, not shell history.
`fetch` requests static assets without authentication. `extract` uses the TypeScript
AST to resolve minified bindings before recovering path, form and operation triples.
`spec` merges those facts with manually verified fields in `overlay.json`.

Sampling authenticates, issues only allowlisted `read` and `load` calls, and logs out.
It excludes firmware, recovery, diagnostics and other namespaces where a read can
start an action. Payloads can contain credentials; sampling infers types in memory,
discards payloads, and saves only schema shapes. Never commit the downloaded bundles,
sampled payloads, or `.cache` directory.

A static run refuses to overwrite a sampled committed spec without its schema cache.
Use the sampled target to populate that cache. Do not overlap this command with a
browser session or deployment. API recovery is not run by CI; `make tools-check`
typechecks the tooling offline.
