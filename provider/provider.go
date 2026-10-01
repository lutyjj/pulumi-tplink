// Package provider implements the TP-Link Archer Pulumi provider.
package provider

import (
	"fmt"
	"github.com/lutyjj/pulumi-tplink/internal/router"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// Version is stamped by the linker at build time.
var Version = "0.0.0-dev"

// Name identifies the package and plugin.
const Name = "tplink"

// Provider constructs a fresh provider instance.
func Provider() p.Provider {
	result, err := infer.NewProviderBuilder().
		WithDisplayName("TP-Link Archer").
		WithLicense("Apache-2.0").
		WithLogoURL("https://raw.githubusercontent.com/lutyjj/pulumi-tplink/main/docs/logo.svg").
		WithDescription("Unofficial provider for TP-Link Archer routers using their local web API.").
		WithKeywords("tplink", "tp-link", "archer", "router", "dhcp", "category/network", "kind/native").
		WithHomepage("https://github.com/lutyjj/pulumi-tplink").
		WithRepository("https://github.com/lutyjj/pulumi-tplink").
		WithPublisher("lutyjj").
		WithNamespace("lutyjj").
		WithSupportPack(true).
		WithPluginDownloadURL("github://api.github.com/lutyjj/pulumi-tplink").
		WithLanguageMap(map[string]any{
			"nodejs": map[string]any{"packageName": "@lutyjj/pulumi-tplink", "respectSchemaVersion": true},
			"python": map[string]any{"respectSchemaVersion": true},
			"go":     map[string]any{"respectSchemaVersion": true},
			"csharp": map[string]any{"respectSchemaVersion": true},
			"java":   map[string]any{"basePackage": "com.lutyjj", "buildFiles": "gradle"},
		}).
		WithGoImportPath("github.com/lutyjj/pulumi-tplink/sdk/go/tplink").
		WithConfig(infer.Config(&Config{})).
		WithResources(
			infer.Resource(DhcpReservation{}), infer.Resource(DhcpServer{}),
			infer.Resource(InboundRules{}), infer.Resource(Upnp{toggle{router.UPnP}}),
			infer.Resource(Dmz{toggle{router.DMZ}}), infer.Resource(RemoteAdmin{toggle{router.RemoteAdmin}}),
			infer.Resource(EasyMesh{toggle{router.EasyMesh}}), infer.Resource(MediaSharing{toggle{router.MediaSharing}}),
			infer.Resource(NatAlg{}), infer.Resource(ParentalControlProfile{}),
		).
		WithModuleMap(map[tokens.ModuleName]tokens.ModuleName{"provider": "index"}).Build()
	if err != nil {
		panic(fmt.Errorf("build provider: %w", err))
	}
	return result
}
