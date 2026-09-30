package provider

import (
	"context"
	"errors"
	p "github.com/pulumi/pulumi-go-provider"
	"net/url"
	"strings"

	"github.com/pulumi/pulumi-go-provider/infer"

	"github.com/lutyjj/pulumi-tplink/internal/router"
)

// Config is the provider-level connection to one router.
type Config struct {
	Host         *string `pulumi:"host,optional"`
	Password     *string `pulumi:"password,optional" provider:"secret"`
	Insecure     *bool   `pulumi:"insecure,optional"`
	routerClient *router.Client
}

// Annotate documents the configuration and wires its environment fallbacks.
func (c *Config) Annotate(a infer.Annotator) {
	a.Describe(&c.Host, "The router's LAN address, with an optional port. May also be set with the TPLINK_HOST environment variable.")
	a.SetDefault(&c.Host, nil, "TPLINK_HOST")
	a.Describe(&c.Password, "The router's local admin password, as typed into its web UI (not a TP-Link ID). May also be set with the TPLINK_PASSWORD environment variable.")
	a.SetDefault(&c.Password, nil, "TPLINK_PASSWORD")
	a.Describe(&c.Insecure, "Skip TLS certificate verification. Routers serve a self-signed certificate, so this is usually required. May also be set with the TPLINK_INSECURE environment variable. Defaults to false.")
	a.SetDefault(&c.Insecure, false, "TPLINK_INSECURE")
}

// Configure rejects a configuration that cannot reach a router.
func (c *Config) Configure(context.Context) error {
	if c.Host == nil || *c.Host == "" {
		return errors.New("tplink: `host` is required (or set TPLINK_HOST)")
	}
	if c.Password == nil || *c.Password == "" {
		return errors.New("tplink: `password` is required (or set TPLINK_PASSWORD)")
	}
	u, err := url.Parse("https://" + *c.Host)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(*c.Host, " \t\n") {
		return errors.New("tplink: host must be a hostname or IP address with an optional port, not a URL")
	}
	c.routerClient = router.New(router.Config{Host: c.host(), Password: deref(c.Password), Insecure: c.Insecure != nil && *c.Insecure})
	return nil
}

// Diff replaces only when the router identity changes. Credential rotation must not
// cascade into destructive replacement of reservations.
func (Config) Diff(_ context.Context, req infer.DiffRequest[*Config, *Config]) (infer.DiffResponse, error) {
	diff := map[string]p.PropertyDiff{}
	if deref(req.State.Host) != deref(req.Inputs.Host) {
		diff["host"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if deref(req.State.Password) != deref(req.Inputs.Password) {
		diff["password"] = p.PropertyDiff{Kind: p.Update}
	}
	oldInsecure := req.State.Insecure != nil && *req.State.Insecure
	newInsecure := req.Inputs.Insecure != nil && *req.Inputs.Insecure
	if oldInsecure != newInsecure {
		diff["insecure"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(diff) > 0, DetailedDiff: diff}, nil
}

func (c Config) host() string { return deref(c.Host) }

// client builds the router client for this configuration.
func (c Config) client() *router.Client { return c.routerClient }

// connection returns the configured router's client and address.
func connection(ctx context.Context) (*router.Client, string) {
	cfg := infer.GetConfig[Config](ctx)
	return cfg.client(), cfg.host()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nonEmpty turns an empty router field into an unset optional.
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
