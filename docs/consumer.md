# Use a development build

Generate the SDK and binary from the same checkout and version:

```sh
make check schema sdk-nodejs VERSION=0.1.0-dev
```

For a sibling TypeScript consumer, declare the generated package as a file dependency:

```json
{
  "dependencies": {
    "@lutyjj/pulumi-tplink": "file:../pulumi-tplink/sdk/nodejs"
  }
}
```

Mount both checkouts into the consumer's Node and Pulumi containers at paths that
preserve that relative dependency. Install the compiled binary into the consumer's
Pulumi plugin cache from its mounted location:

```sh
pulumi plugin install resource tplink 0.1.0-dev \
  --file /pulumi-tplink/bin/pulumi-resource-tplink --reinstall
```

This command changes the plugin cache, not managed devices. Reinstall after rebuilding
an unchanged development version. For a released version, use Pulumi's normal package installation instead:

```sh
pulumi package add tplink@0.1.0 --server github://api.github.com/lutyjj/pulumi-tplink
```

Choose a version present on the repository's Releases page. This generates the
binding for the current project's language without requiring a published language
package. Private release assets require an authorized `GITHUB_TOKEN`.
There are no release assets until the operator publishes a release.

## Example

```ts
import * as pulumi from "@pulumi/pulumi";
import * as tplink from "@lutyjj/pulumi-tplink";

const config = new pulumi.Config();
const router = new tplink.Provider("router", {
    host: config.require("routerHost"),
    password: config.requireSecret("routerPassword"),
    insecure: true,
});
const options = { provider: router, protect: true };

new tplink.DhcpServer("dhcp", {
    primary: "192.0.2.53",
    poolStart: "192.0.2.100",
    poolEnd: "192.0.2.199",
}, options);
new tplink.DhcpReservation("printer", {
    mac: "02:00:00:00:00:01",
    ip: "192.0.2.20",
    deviceName: "printer",
}, options);
new tplink.Upnp("upnp", { enabled: false }, options);
new tplink.Dmz("dmz", { enabled: false }, options);
new tplink.RemoteAdmin("remote-admin", { enabled: false }, options);
```

The example addresses are reserved documentation addresses. Set real connection
values with `pulumi config set routerHost <address>` and
`pulumi config set --secret routerPassword <password>`.

Declaring `InboundRules` additionally removes all port forwards and triggers on apply.
That is intentionally destructive to those tables; declare it only when that policy
is wanted. Preview before apply, and do not overlap browser or deployment sessions.
