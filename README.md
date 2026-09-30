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

## Protocol and API recovery

[Protocol notes](docs/protocol.md) explain the RSA-only login and positional deletion.
`api/openapi.json` records the endpoint surface recovered from the firmware web UI;
it is not the Pulumi resource schema. The Go client implements only the calls the
resources use, rather than generating thousands of unused endpoint methods.

The [recovery tooling](tools/codegen/README.md) fetches and analyses web bundles in a
container. Its caches never enter Git. Sampling persists field names and types only,
not payloads or secrets.

## Releases and public-package readiness

CI validates source, checks schema drift and compiles the Node SDK. A version-tag
push runs the separate release workflow, producing Pulumi plugin archives for Linux,
macOS and Windows on amd64 and arm64. Private release assets require GitHub credentials.
No SDK registry upload runs, and no release has been made merely by pushing `main`.

Before making this public: choose a license, review firmware-derived material and
trademarks, cut a tested release, add a logo, and submit the package to the Pulumi
Registry. The overview is prepared in `docs/_index.md`; listing is not automatic.
