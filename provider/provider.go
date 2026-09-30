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
		WithDescription("Unofficial provider for TP-Link Archer routers using their local web API.").
		WithKeywords("tplink", "tp-link", "archer", "router", "dhcp", "category/network", "kind/native").
		WithHomepage("https://github.com/lutyjj/pulumi-tplink").
		WithRepository("https://github.com/lutyjj/pulumi-tplink").
		WithPublisher("lutyjj").
		WithNamespace("lutyjj").
		WithPluginDownloadURL("github://api.github.com/lutyjj/pulumi-tplink").
		WithLanguageMap(map[string]any{"nodejs": map[string]any{"packageName": "@lutyjj/pulumi-tplink"}}).
		WithGoImportPath("github.com/lutyjj/pulumi-tplink/sdk/go/tplink").
		WithConfig(infer.Config(&Config{})).
		WithResources(
			infer.Resource(DhcpReservation{}), infer.Resource(DhcpServer{}),
			infer.Resource(InboundRules{}), infer.Resource(Upnp{toggle{router.UPnP}}),
			infer.Resource(Dmz{toggle{router.DMZ}}), infer.Resource(RemoteAdmin{toggle{router.RemoteAdmin}}),
		).
		WithModuleMap(map[tokens.ModuleName]tokens.ModuleName{"provider": "index"}).Build()
	if err != nil {
		panic(fmt.Errorf("build provider: %w", err))
	}
	return result
}
