# Automated API discovery

Status: research proposal. No discovery engine or generated router client is
implemented by this document.

Build a maintainer tool that discovers router operations, records evidence and
produces a typed Go client from reviewed contracts. Keep it out of provider
startup and user deployments. A discovery result is not permission to call an
endpoint, and a generated method is not proof that an operation is safe.

## Findings that shape the design

The router multiplexes operations through query and form fields. For example,
`POST /admin/dhcps?form=reservation` with `operation=load` and the same request
with `operation=remove` have different contracts. The authenticated URL also
contains a session token, which is runtime state, not part of operation identity.

[OpenAPI identifies an operation by HTTP method and path](https://swagger.io/docs/specification/v3_0/paths-and-operations/).
It cannot give those two requests separate operations, and query strings cannot
be included in its path keys. A faithful OpenAPI document can group them under one
POST operation, but that does not generate the separate typed methods we want.
Do not invent paths and silently rewrite them in a transport adapter.

Use a small operation catalog for routing and safety metadata, with JSON Schema
for request and response shapes. Generate the Go wire methods from this catalog.
Do not require an OpenAPI export for the first implementation.

A disposable parser probe used
[`tdewolff/parse/v2` v2.8.16](https://github.com/tdewolff/parse/tree/v2.8.16/js),
a Go parser with an MIT license. It parsed four modules served by the verified
router's web UI without executing JavaScript:

| Module role | Bytes | Static imports | Call expressions | Endpoint-shaped string literals |
| --- | ---: | ---: | ---: | ---: |
| Entry and routes | 25,457 | 4 | 446 | 48 |
| Shared services and UI support | 888,440 | 2 | 11,190 | 51 |
| Login | 19,189 | 26 | 547 | 6 |
| IoT isolation | 4,310 | 6 | 114 | 4 |

These are syntax counts, not a count of supported or unique API operations.
Synthetic probes also passed for module aliases, literal dynamic imports,
optional calls and malformed-input rejection. Runtime-computed targets remain
unresolved. This establishes parser suitability for the sample, not complete
extraction. No authenticated requests or router writes were made by this probe.
Raw modules and the temporary probe are not committed.

## Proposed layout

Add files when their implementation milestone needs them, not as empty scaffolding.
Use the existing module and Go toolchain; do not introduce a second authored language.

```text
internal/
  discovery/
    cmd/router-discovery/main.go   command parsing and exit status
    acquire.go                     bounded, same-origin asset acquisition
    extract.go                     JavaScript AST and binding analysis
    catalog.go                     catalog types and validation
    generate.go                    deterministic Go source generation
    observe.go                     explicitly authorized read probes
    *_test.go
    testdata/                      synthetic JavaScript and wire fixtures
  router/
    client.go                      authentication, sessions and HTTP bounds
    forms.go                       envelope and form/table decoding
    dhcp.go, settings.go, ...       merge, identity and recovery semantics
    operations_gen.go              generated request/response types and wire calls
    spec/
      archer-rsa.json               reviewed operation and device catalog
  routertest/                       independent fake-router behavior
provider/                          Pulumi lifecycles and public schema
.cache/discovery/                  private snapshots and candidate reports; ignored
```

Keep discovery in one package initially. Its files separate responsibilities;
they do not need interfaces, factories or plugin registries. Extract another
package only when a real consumer or dependency boundary requires it.

Dependencies point from the discovery command to discovery, and from the optional
observer to the existing router client. The router client never imports discovery.
The provider uses the router package, not the catalog parser or JavaScript parser.
Generated wire methods stay in package `router` so they can use the existing
session machinery without exposing an unrestricted request API.

## Pipeline and ownership

### 1. Acquire an immutable snapshot

Accept an explicit device target. Fetch the HTML entry point and follow static
imports, re-exports, module preloads and literal dynamic imports. Do not evaluate
bundles, run browser actions or guess API calls. Report computed imports as gaps.

Allow only the selected origin and asset paths. Refuse redirects, traversal and
symlink escapes; pin the approved network destination across DNS resolution.
Apply byte limits before and after decompression, plus total asset, depth and time
budgets. Report every skipped dependency and limit reached. Assets use content
hashes as filenames, not URL-derived filesystem paths.

Store files privately under an ignored snapshot directory. Record asset hashes,
import edges, entry point and tool version in a manifest. Keep UI build identity
separate from authenticated model, hardware, firmware and region information.
Unauthenticated snapshots must mark firmware identity unknown, not infer it from
HTML. Finish in a temporary directory and rename it only after the manifest is
complete. Cancellation leaves the last complete snapshot intact.

### 2. Extract candidates offline

Parse module ASTs and resolve bindings by scope and import/export identity, not
minified variable names. Start with immutable string constants, simple literal
concatenation, adapter aliases and explicit object properties. Learn the known
adapter's `read`, `load`, `write`, `insert`, `update` and `remove` call shapes from
source. Record their encoding rules; do not assume names alone establish behavior.

Retain provenance: asset digest, module binding and AST location. Report unresolved
expressions, unrecognized adapters and unsupported syntax rather than executing
JavaScript or silently skipping them. Report acquisition coverage separately from
resolved calls and known response shapes. Regex hits can aid exploration but are
not the operation catalog.

Produce a candidate report, never overwrite the reviewed catalog. An endpoint
literal, a resolved call and a tested operation are separate facts.

### 3. Review the contract

The catalog is the source of truth for generated wire behavior. Each operation
has a stable ID, HTTP method, relative path, fixed query and form selectors,
parameter encoding, request/response schema references, effect classification and
evidence references. The identity includes `form` and `operation`, not just path.
Keep stable IDs independent of bundle filenames and JavaScript variable names.

Use local JSON Schema definitions for payload shapes. Declare requiredness, nulls,
wire string/number distinctions, unknown-field policy and JSON embedded in form
values explicitly. Preserve the router's success/error envelope. Empty tables
must not be interpreted as evidence for a populated row schema.

Initially generate only the schema constructs required by the first client slice.
Reject unsupported constructs and unresolved references; do not silently emit
`any` or discard unknown fields needed for read/merge/write operations. Use Go's
`text/template` and `go/format` for the thin wire generator, with validated names
and escaped literals. Reassess a general model generator only when a concrete
schema cannot be handled without recreating one.

A maintainer reviews candidate changes and accepts them into the catalog. New
fields observed in one response do not automatically become required. Observed
values do not automatically become enums. Missing calls in a new snapshot do not
automatically remove supported operations.

### 4. Observe only approved operations

Live observation is a separate, explicit command. Its allowlist is reviewed and
independent of discovery output. It authorizes the complete request tuple and
parameter constraints, not every POST or every operation named `read`.
Discovered request parameters cannot override routing or operation selectors.
Unknown operations are denied. Login/logout are the observer's only automatic
session mutations; forced login requires separate permission.

Reuse the existing serialized session and timeout behavior. Keep credentials in
memory and out of arguments, manifests and logs. Do not persist raw HAR files or
API responses. Record structural evidence and explicitly permitted metadata;
replace personal values with synthetic fixtures before writing test data. Dynamic
object keys can contain personal data too. Fail closed when safe projection is
undefined. An error must not echo a session-bearing URL or raw response.

Do not probe firmware installation, resets, reboots, WAN, radio or routing changes
against an ordinary user's router. Tests of writes require a named test target,
explicitly bounded changes, recovery preparation and post-write verification.

### 5. Generate and verify offline

Generate one `operations_gen.go` containing the accepted types and wire methods.
Sort output, omit timestamps and host identity, and embed the catalog digest.
Validate and format a temporary file before replacing the generated file.
A check mode compares output without changing the working tree.

Commit the reviewed catalog and generated Go source together. Unlike consumer
SDKs, this generated source is compiled into the provider. Consumers and ordinary
builds must not need a router, private snapshot or generator to compile it.

Authentication and session cleanup remain handwritten. So do semantic safeguards:
merge-preserving writes, fresh positional lookup, rollback and Pulumi lifecycle
rules. Generate transport mechanics, not guesses about resource ownership.
When a generated method replaces a manual call, remove the obsolete call in the
same change. Do not leave dual clients, fallback implementations or unused methods.

## Device support

Group devices by verified protocol behavior rather than duplicating a client for
every model. Start with the single verified RSA profile. Its catalog records
model, hardware, region, firmware evidence and verification per operation.
Authentication, API shape and write semantics can each change independently.

Keep these evidence dimensions distinct: static discovery, observed responses,
reviewed contract, successful generation, fake-server tests, live reads and live
writes. Passing one does not imply another. Generated clients must not claim
support for unknown operations merely because the UI mentions them.

Use the catalog to derive the compatibility table and, when runtime selection is
implemented, its compiled profile metadata. A model override must not bypass
identity or capability checks. Choose authentication from an explicit known
profile or documented unauthenticated metadata; do not try multiple password
protocols until one works. Unknown devices get an actionable diagnostic, not an
automatic write attempt. Add another profile only with evidence of its differences.

## Implementation milestones

1. **Offline extraction:** acquire and parse a bounded asset graph; report resolved
   request tuples and unresolved calls. No authenticated probing or generated code.
   Synthetic tests cover scope shadowing, alias/re-export resolution, computed
   imports, cycles, duplicate candidates, malformed input and interrupted snapshots.
2. **One consumed client slice:** model DHCP settings reads, generate their wire
   call, and use it from the existing validated `ReadDHCPServer` path. Test exact
   method, URL selectors, encoded body, envelope errors and retained fields against
   an independently specified fake server. Regeneration must produce no diff.
3. **Existing-resource parity:** migrate the wire calls used by the six resources
   without changing their Pulumi schema or behavior. Keep regression tests for
   index deletion, full-record writes, disabled reservations and rollback. Remove
   replaced manual request construction as each slice is adopted.
4. **Opt-in observation and compatibility reporting:** add the reviewed read
   allowlist, private structural evidence and device comparison. Live write tests
   remain separate. Additional API features need their own resource design and scope.

Wire new commands into Make and the pinned Compose tools. Existing `make check`
already covers `internal/...`; add regeneration checks and offline fixtures to CI
when their consumers exist. CI must not have router credentials or LAN access.
Do not add unused scripts, an all-purpose discovery framework, or a restored API
dump that nothing consumes.

## Review and acceptance

Apply architecture, consistency, simplification, security, tests and external
contribution guidelines during implementation:

- One catalog owns wire shapes; generated callers and compatibility docs must agree.
- Untrusted bundles cannot execute code, redirect downloads or authorize probes.
- Fixtures assert exact known requests and failure behavior, not generator output
  copied back as expected values. Fuzz parsing and catalog validation with bounds.
- Changes in types, selectors, effects or verification status appear in reviewed
  diffs. Unsupported cases fail visibly rather than lowering validation.
- Preserve the existing provider's tests, schema and user behavior at parity.
- Review outgoing files for secrets and vendor-source redistribution. Keep full
  firmware bundles private; commit synthetic snippets and derived contracts only.

The parser probe and this design are a self-assessment, not an independent review
or merge approval. Before merging implementation, obtain the independent specialist
review required by the repository's review process. Do not launch review agents,
open a PR, publish a release or modify a router without the relevant authorization.
