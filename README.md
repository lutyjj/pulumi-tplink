# pulumi-tplink

An unofficial native Pulumi provider for TP-Link Archer routers. It manages DHCP
reservations, DHCP DNS and pool bounds, UPnP, DMZ, WAN administration, and the absence
of inbound NAT rules through the router's local web API. It needs no cloud account,
SSH access, or Terraform bridge.

Tested protocol: Archer AXE75, firmware 1.10.5. Other routers and firmware versions
are unverified. This repository is private; it has no public registry listing or
published language SDKs. No open-source license has been selected yet.

## Develop

The host needs Git, make and Docker Compose. All toolchains run in pinned
containers. Go dependencies are locked by `go.mod` and `go.sum`.

```sh
make check schema sdk-nodejs
```

`make schema` derives `provider/cmd/pulumi-resource-tplink/schema.json` from the Go
resource types. `make sdk-nodejs` generates and compiles the TypeScript SDK for
sibling-checkout consumers. `make sdks` generates the other Pulumi language SDKs.
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
The Node toolchain only compiles the generated TypeScript SDK. Schema export uses
Pulumi's CLI and jq, following the native-provider template.

[Protocol notes](docs/protocol.md) explain the RSA-only login, verified endpoints and
positional deletion. The provider implements only the calls its resources need.

## Releases and public-package readiness

CI validates source, checks schema drift and compiles the Node SDK. A version-tag
push runs the separate release workflow, producing Pulumi plugin archives for Linux,
macOS and Windows on amd64 and arm64. Private release assets require GitHub credentials.
No SDK registry upload runs, and no release has been made merely by pushing `main`.

Before making this public: choose a license, review the protocol implementation and
trademarks, cut a tested release, add a logo, and submit the package to the Pulumi
Registry. The overview is prepared in `docs/_index.md`; listing is not automatic.
