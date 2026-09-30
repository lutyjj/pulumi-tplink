# pulumi-tplink

An unofficial native Pulumi provider for TP-Link Archer routers. It manages DHCP
reservations, DHCP DNS and pool bounds, UPnP, DMZ, WAN administration, and the absence
of inbound NAT rules through the router's local web API. It needs no cloud account,
SSH access, or Terraform bridge.

Tested protocol: Archer AXE75, firmware 1.10.5. Other routers and firmware versions
are unverified. Licensed under [Apache-2.0](LICENSE). This project is not affiliated
with or endorsed by TP-Link or Pulumi. There is no published release or Registry
listing yet; development builds are usable through generated SDKs.

## Develop

The host needs Git, make and Docker Compose. All toolchains run in pinned
containers. Go dependencies are locked by `go.mod` and `go.sum`.

```sh
make check sdk-check release-check secret-check
```

`make schema` derives `provider/cmd/pulumi-resource-tplink/schema.json` from the Go
resource types. `make sdk-nodejs` generates and compiles the TypeScript SDK for
sibling-checkout consumers. `make sdks` generates the other Pulumi language SDKs.
`make sdk-check` generates and compiles Node.js, Python, Go, .NET and Java bindings,
including an installed Python import check. `make release-check` builds all six
plugin archives and loads the native archive through an isolated Pulumi plugin cache.
`make secret-check` scans Git history and the working diff with Gitleaks.
SDKs, binaries and caches are build outputs, not committed sources.

The default development version is `0.1.0-dev`. Set `VERSION=x.y.z` to stamp a build.
A sibling consumer must install the binary with the same version as its generated SDK.
See [consumer setup](docs/consumer.md).

## Resources

| Resource | Contract on destroy |
| --- | --- |
| `DhcpReservation` | Deletes the reservation; keeps the displayed device name |
| `DhcpServer` | Keeps DHCP configuration unchanged |
| `Upnp`, `Dmz`, `RemoteAdmin` | Keeps the toggle unchanged |
| `InboundRules` | Stops asserting empty NAT tables; never restores removed rules |

`InboundRules` reads both tables during diff, even without a refresh. Applying it
removes every port forward and port trigger it finds. Preview logs those rules and
does not remove them. Other resources report drift on refresh.

Connection settings belong to an explicit `Provider`: `host`, secret `password`,
and optional `insecure` (default false). `TPLINK_HOST`, `TPLINK_PASSWORD` and
`TPLINK_INSECURE` provide environment fallbacks. `host` is an address with an optional
port, not a URL. `insecure: true` explicitly accepts the router's self-signed TLS
certificate. Sessions serialize within a provider process, not across deployments or
browser logins. Do not overlap those operations.

## Layout

| Path | Owns |
| --- | --- |
| `internal/router/` | Go transport, authentication and router operations |
| `provider/` | Go Pulumi configuration and resource lifecycles |
| `provider/cmd/pulumi-resource-tplink/` | Plugin entry point and generated Pulumi schema |
| `internal/routertest/` | Fake router for focused tests |
| `sdk/` | Generated consumer bindings; ignored by Git |
| `bin/`, `dist/`, `.cache/` | Build outputs and tool caches; ignored by Git |
| `Makefile`, `compose.yaml` | Containerised build, schema export, SDK generation and releases |

All authored implementation code is Go. Pulumi generates the SDKs for consumers in
other languages; they are bindings to the same plugin, not separate implementations.
Language-specific containers generate and compile consumer SDKs. Schema export uses
Pulumi's CLI and jq, following the native-provider template. SDK versions come from
the stamped plugin schema, including language-specific version conversions.

[Protocol notes](docs/protocol.md) explain the RSA-only login, verified endpoints and
positional deletion. The provider implements only the calls its resources need.

## Releases

CI validates provider source, schema parity, secret scanning, release installation,
and each language SDK in separate jobs. A version-tag push runs those gates before
publishing plugin archives for Linux, macOS and Windows on amd64 and arm64.
It does not upload SDKs to language registries.

After a release is available, run this inside your Pulumi project, replacing the
version with the release you selected:

```sh
pulumi package add tplink@0.1.0 --server github://api.github.com/lutyjj/pulumi-tplink
```

Pulumi generates the binding for your project's language. YAML consumes the schema
directly. Published npm, PyPI, NuGet, Maven and Go packages are not required for this
installation path. Private release assets require an authorized `GITHUB_TOKEN`.
For an unreleased checkout, follow [consumer setup](docs/consumer.md).

Opening the repository, pushing a release tag and submitting it to the Pulumi
Registry are separate operator actions. Registry registration requires an entry in
`pulumi/registry`'s `community-packages/package-list.json` pointing to this repository
and `provider/cmd/pulumi-resource-tplink/schema.json`, plus the `lutyjj` publisher
mapping if it does not exist. The Registry overview is `docs/_index.md`.

To update toolchains, change each Compose image's tag and digest together, then run
the checks above. Go module versions are locked; generated SDK dependency ranges
are resolved by their own language tools during validation.
