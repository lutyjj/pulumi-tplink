# Agent guidelines

This repo owns an unofficial native Pulumi provider for TP-Link Archer routers.
Repository visibility and releases require explicit operator approval.
Distribute through GitHub releases, not the Pulumi Registry or language registries.

- Treat this project as greenfield. No deprecated paths, compatibility shims,
  state-format adapters, or transitional implementations. Rewrite a unit when extending
  it would make it harder to maintain. Complete replacements in
  one change, removing obsolete code, configuration, callers, tests and documentation.
- Write for a stranger reading the public repository. Keep consumer-specific hostnames,
  addresses, paths, inventory and deployment details out of source and documentation.
  Use reserved example addresses and domains.
- Run all build, lint, generation and test tools through Docker Compose and Make.
- Keep Go code idiomatic. Prefer direct composition and standard library networking.
  Do not build a vendor-agnostic router framework or add unsupported firmware variants.
- All authored implementation code is Go. JavaScript and TypeScript belong only
  in generated consumer SDKs, not provider-side tools.
- `internal/router` owns the wire protocol and session lifecycle. `provider` owns
  Pulumi configuration and resources. Derive schema and SDKs from Go types with
  `pulumi-go-provider`; never hand-edit the generated schema or SDKs.
- Keep parity with the verified Archer AXE75 firmware 1.10.5 behaviour. Preserve
  unmanaged form fields, resolve positional deletes from fresh reads, and serialize
  login-through-logout sessions per router. Preview must never write settings or rules.
- Do not access or change a real router unless the operator delegates that target and
  operation. A provider migration is separate from a configuration change. Preview
  before apply, stop on unexpected replacements, and verify after any approved apply.
- Keep passwords, cookies, tokens, live response payloads and host inventory out of
  source, logs and commits.
- Run `make check` for provider changes, `make sdk-check` for schema or SDK tooling
  changes, and `make release-check secret-check` before publication. Add tests only for meaningful
  lifecycle or wire-boundary behaviour, not boilerplate or coverage targets.
- Commit and push only on request. Stage relevant files, inspect the diff, use
  Conventional Commits, never force-push or skip hooks.
- Release tags, public visibility and router deployments each
  require explicit authorization. A push to `main` runs CI but publishes nothing.

- Use the latest compatible stable dependencies and CI actions. Keep image digests and
  action commit pins alongside readable versions; document compatibility exceptions.
- Keep README focused on users: what it does, installation, first use and updates.
  Use plain, conversational English. Put build details in docs/development.md.
