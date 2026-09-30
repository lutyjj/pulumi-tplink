---
title: TP-Link Archer
meta_desc: Manage TP-Link Archer routers with the unofficial native Pulumi TP-Link provider.
layout: package
---

The TP-Link provider manages Archer routers through their local web API. Resources
cover DHCP reservations, DNS and pool bounds, UPnP, DMZ, WAN administration and empty
inbound NAT tables. The protocol is verified on Archer AXE75 firmware 1.10.5.

## Installation

This package is private and not listed in the public Registry. Build the plugin and
local SDK following the repository's [consumer setup](https://github.com/lutyjj/pulumi-tplink/blob/main/docs/consumer.md).
No SDK has been published to npm or another language registry.

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
