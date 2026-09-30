# Agent guidelines

This repo owns an unofficial native Pulumi provider for TP-Link Archer routers.
The repository stays private until the operator explicitly approves publication.

- Run all build, lint, generation and test tools through the containerised Makefile.
- Keep Go code idiomatic. Prefer direct composition and standard library networking.
  Do not build a vendor-agnostic router framework or add unsupported firmware variants.
- `internal/router` owns the wire protocol and session lifecycle. `provider` owns
  Pulumi configuration and resources. Derive schema and SDKs from Go types with
  `pulumi-go-provider`; never hand-edit the generated schema or SDKs.
- Keep parity with the verified Archer AXE75 firmware 1.10.5 behaviour. Preserve
  unmanaged form fields, resolve positional deletes from fresh reads, and serialize
  login-through-logout sessions per router. Preview must never write settings or rules.
- Do not access or change a real router unless the operator delegates that target and
  operation. A provider migration is separate from a configuration change. Preview
  before apply, stop on unexpected replacements, and verify after any approved apply.
- Keep passwords, cookies, tokens, sampled payloads, downloaded firmware bundles and
  host inventory out of source, logs and commits. Only schema shapes may leave sampling.
- Run focused validation: `make check schema sdk-nodejs`. Add tests only for meaningful
  lifecycle or wire-boundary behaviour, not boilerplate or coverage targets.
- Commit and push only on request. Stage relevant files, inspect the diff, use
  Conventional Commits, never force-push or skip hooks.
- Release tags, registry submissions, public visibility and router deployments each
  require explicit authorization. A push to `main` runs CI but publishes nothing.
