# Development

You need Git, make and Docker Compose. Build tools run in containers, so you don't
need to install Go or each SDK's tools on your host.

```sh
make check sdk-check release-check secret-check
```

| Command | What it checks |
| --- | --- |
| `make check` | Go formatting, vet, lint and tests with the race detector |
| `make schema` | Generates the Pulumi schema from the Go resource types |
| `make sdk-check` | Generates and builds all five SDKs; imports the installed Python package |
| `make release-check` | Builds six release archives and loads the native Linux plugin in an isolated cache |
| `make secret-check` | Scans Git history and the working diff with Gitleaks |

`make help` lists the other commands. SDKs, binaries and caches are ignored build
outputs. Don't edit them by hand.

## Try a development build

Build the provider and TypeScript SDK from the same checkout and version:

```sh
make check schema sdk-nodejs VERSION=0.1.0-dev
```

In a sibling TypeScript project, add the generated SDK as a file dependency:

```json
{
  "dependencies": {
    "@lutyjj/pulumi-tplink": "file:../pulumi-tplink/sdk/nodejs"
  }
}
```

Run your package manager's install command. Then install the matching provider
binary in your Pulumi plugin cache:

```sh
pulumi plugin install resource tplink 0.1.0-dev \
  --file ../pulumi-tplink/bin/pulumi-resource-tplink --reinstall
```

If your project runs in containers, mount both checkouts and keep those relative
paths intact. Reinstall the plugin after rebuilding the same development version.
This changes the plugin cache, not your router. Follow the
[connection example](../README.md#connect-your-router) to use it.

## Find your way around

`internal/router/` handles the router's HTTP API and login sessions. `provider/`
contains Pulumi configuration and resources. The plugin entry point and generated
schema live in `provider/cmd/pulumi-resource-tplink/`. Tests use the fake router in
`internal/routertest/`.

The implementation is Go. Pulumi generates the SDKs from the schema; those SDKs
call the same provider, rather than implementing router operations themselves.

## Dependencies and releases

Use the latest compatible stable releases. Go modules are recorded in `go.mod`
and `go.sum`; Compose images and GitHub Actions use immutable pins with readable
versions. Check upstream releases, update each version and pin together, then run
the checks above. For Go dependency updates, run `go get -u ./...` and `go mod tidy`
through the `go-tools` Compose service. Pulumi owns the generated SDK build files
and dependency defaults.

The Java build uses the latest Gradle 8 patch with JDK 11 because Pulumi generates
a Java 11 toolchain. Gradle 9 needs a newer runtime and a separate Java 11 toolchain.
Keep this exception until the generated build can use it without extra tooling.

CI checks the source, committed schema, SDKs and release archives. Pushing a
`vX.Y.Z` tag runs validation before publishing the plugin archives and checksums
to GitHub Releases. It doesn't publish language packages or deploy router changes.
Repository visibility and release publication are separate, explicit decisions.
