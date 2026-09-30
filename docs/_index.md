---
title: TP-Link Archer
meta_desc: Manage TP-Link Archer routers with the unofficial native Pulumi TP-Link provider.
layout: package
---

The TP-Link provider manages Archer routers through their local web API. Resources
cover DHCP reservations, DNS and pool bounds, UPnP, DMZ, WAN administration and empty
inbound NAT tables. The protocol is verified on Archer AXE75 firmware 1.10.5.

## Installation

Choose a version available on the repository's
[Releases page](https://github.com/lutyjj/pulumi-tplink/releases), then run inside your
Pulumi project:

```sh
pulumi package add tplink@0.1.0 --server github://api.github.com/lutyjj/pulumi-tplink
```

Replace `0.1.0` with the selected release. Pulumi generates a typed binding for
TypeScript/JavaScript, Python, Go, .NET or Java. YAML uses the schema directly;
no published language package is required. For an unreleased checkout, follow the
[consumer setup](https://github.com/lutyjj/pulumi-tplink/blob/main/docs/consumer.md).

## Example usage

```ts
import * as pulumi from "@pulumi/pulumi";
import * as tplink from "@lutyjj/pulumi-tplink";

const config = new pulumi.Config();
const router = new tplink.Provider("router", {
    host: config.require("routerHost"),
    password: config.requireSecret("routerPassword"),
    insecure: true,
});

new tplink.DhcpReservation("printer", {
    mac: "02:00:00:00:00:01",
    ip: "192.0.2.20",
    deviceName: "printer",
}, { provider: router });
```

```sh
pulumi config set routerHost <router-address>
pulumi config set --secret routerPassword <local-admin-password>
```

## Configuration

| Name | Required? | Secret? | Description |
| --- | --- | --- | --- |
| `host` | Yes | No | Router address with optional port, or `TPLINK_HOST` |
| `password` | Yes | Yes | Local admin password, or `TPLINK_PASSWORD`; not a TP-Link ID |
| `insecure` | No | No | Skip TLS verification, or `TPLINK_INSECURE`; defaults to false |

The router permits a single admin session. Provider operations serialize within a
process, but separate deployments and browser logins must not overlap. The
`InboundRules` resource removes every port forward and trigger on apply; use it only
when that policy is intentional.
