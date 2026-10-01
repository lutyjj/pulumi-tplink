# pulumi-tplink

Manage your TP-Link Archer router with [Pulumi](https://www.pulumi.com/).
Reserve IP addresses, set DHCP and DNS options, and control UPnP, DMZ, remote
administration, EasyMesh, media sharing and NAT ALGs from your Pulumi project. The provider talks directly to your router.
You don't need a TP-Link cloud account or SSH access.

This is an unofficial project. API reads are verified on **Archer AXE75 v1.0**
with **firmware 1.5.6**. See the [compatibility notes](docs/protocol.md#verified-device)
for the tested build and limits. Other models and firmware versions haven't been tested.

## Install

You'll need a [Pulumi project](https://www.pulumi.com/docs/iac/get-started/),
its language's tools, and network access to your router.

Choose a version from [GitHub Releases](https://github.com/lutyjj/pulumi-tplink/releases).
Run this in your project, replacing `0.1.0` with that version:

```sh
pulumi package add tplink@0.1.0 \
  --server github://api.github.com/lutyjj/pulumi-tplink
```

Pulumi downloads the provider and generates the SDK for your project's language.
TypeScript, JavaScript, Python, Go, C# and Java are supported; Pulumi YAML uses the
provider directly. You don't need to build the provider or install a separate
package from a language registry. Releases include Linux, macOS and Windows binaries
for amd64 and arm64. Private releases need an authorized `GITHUB_TOKEN`.

## Connect your router

Save the router's address and local admin password. Leave the password out of the
command so Pulumi prompts for it:

```sh
pulumi config set routerHost <router-address>
pulumi config set --secret routerPassword
```

Here's a TypeScript example that reserves an address for a printer:

```ts
import * as pulumi from "@pulumi/pulumi";
import * as tplink from "@lutyjj/pulumi-tplink";

const config = new pulumi.Config();
const router = new tplink.Provider("router", {
    host: config.require("routerHost"),
    password: config.requireSecret("routerPassword"),
    insecure: true, // Only if you accept the router's self-signed certificate.
});

new tplink.DhcpReservation("printer", {
    mac: "02:00:00:00:00:01",
    ip: "192.0.2.20",
    deviceName: "printer",
}, { provider: router, protect: true });
```

Replace the example MAC and IP with your device's values. Use a hostname or IP
address for `host`, optionally with a port, but without `https://`.
TLS verification is on by default; `insecure: true` turns it off.
You can also supply provider settings through `TPLINK_HOST`, `TPLINK_PASSWORD`
and `TPLINK_INSECURE`.

Run `pulumi preview --refresh` and check the changes before running `pulumi up`.
The router only allows one admin session, so don't use its web interface or run
another deployment at the same time. The example protects the reservation from
accidental deletion; remove `protect` deliberately if you want to delete it.

## What you can manage

| Resource | What it does | When you destroy it |
| --- | --- | --- |
| `DhcpReservation` | Reserves an IP for a MAC address | Deletes the reservation, but keeps the device name |
| `DhcpServer` | Sets DHCP DNS servers and pool bounds | Leaves the settings alone |
| `Upnp`, `Dmz`, `RemoteAdmin`, `EasyMesh`, `MediaSharing` | Turns the setting on or off | Leaves the setting alone |
| `NatAlg` | Turns each NAT ALG and VPN passthrough on or off | Leaves the settings alone |
| `InboundRules` | Removes all port forwards and port triggers | Leaves the tables alone; doesn't restore rules |

**Only add `InboundRules` if you want every port forward and trigger removed.**
Its preview lists the rules it would remove, including on first use.
Preview doesn't change router settings. Use refresh to detect outside changes to
other resources.

## Update

Run the install command again with the release version you want, then review
`pulumi preview --refresh` before applying. Updates are your choice; publishing a
release doesn't update your project or router.

## Contribute

See [development](docs/development.md) for builds and tests, and
[protocol notes](docs/protocol.md) for the router API and its limits.
If you're reporting a problem, include your router model, firmware and provider
version. Don't include passwords, session tokens or raw router responses.

## License

[Apache-2.0](LICENSE). This project isn't affiliated with or endorsed by TP-Link
or Pulumi.
