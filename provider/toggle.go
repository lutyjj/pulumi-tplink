package provider

import (
	"context"

	"github.com/pulumi/pulumi-go-provider/infer"

	"github.com/lutyjj/pulumi-tplink/internal/router"
)

// ToggleArgs are the inputs of a capability that is simply on or off.
type ToggleArgs struct {
	Enabled bool `pulumi:"enabled"`
}

// ToggleState is the recorded state of a toggle.
type ToggleState struct {
	ToggleArgs
}

// toggle implements a capability the firmware models as one `enable` field. Each
// concrete resource embeds it with its own endpoint; the vendor's on/off spelling
// stops here.
//
// Destroying a toggle stops asserting it. A setting has no absent state, and nothing
// records what it held before, so there is nothing to restore.
type toggle struct {
	target router.Toggle
}

func (t toggle) id(host string) string { return host + "/" + t.target.Path + "?form=" + t.target.Form }

func (t toggle) set(ctx context.Context, enabled bool) error {
	client, _ := connection(ctx)
	return client.Do(ctx, func(s *router.Session) error { return s.SetToggle(ctx, t.target, enabled) })
}

// Create asserts the toggle.
func (t toggle) Create(
	ctx context.Context, req infer.CreateRequest[ToggleArgs],
) (infer.CreateResponse[ToggleState], error) {
	_, host := connection(ctx)
	out := infer.CreateResponse[ToggleState]{ID: t.id(host), Output: ToggleState{req.Inputs}}
	if req.DryRun {
		return out, nil
	}
	return out, t.set(ctx, req.Inputs.Enabled)
}

// Update asserts the new value.
func (t toggle) Update(
	ctx context.Context, req infer.UpdateRequest[ToggleArgs, ToggleState],
) (infer.UpdateResponse[ToggleState], error) {
	out := infer.UpdateResponse[ToggleState]{Output: ToggleState{req.Inputs}}
	if req.DryRun {
		return out, nil
	}
	return out, t.set(ctx, req.Inputs.Enabled)
}

// Read reports the value the router holds.
func (t toggle) Read(
	ctx context.Context, req infer.ReadRequest[ToggleArgs, ToggleState],
) (infer.ReadResponse[ToggleArgs, ToggleState], error) {
	client, _ := connection(ctx)
	var enabled bool
	err := client.Do(ctx, func(s *router.Session) (err error) {
		enabled, err = s.ReadToggle(ctx, t.target)
		return err
	})
	if err != nil {
		return infer.ReadResponse[ToggleArgs, ToggleState]{}, err
	}
	args := ToggleArgs{Enabled: enabled}
	return infer.ReadResponse[ToggleArgs, ToggleState]{ID: req.ID, Inputs: args, State: ToggleState{args}}, nil
}

// Delete stops asserting the toggle and leaves the router as it is.
func (toggle) Delete(context.Context, infer.DeleteRequest[ToggleState]) (infer.DeleteResponse, error) {
	return infer.DeleteResponse{}, nil
}

// Upnp asserts whether UPnP is enabled.
type Upnp struct{ toggle }

// Annotate describes the resource.
func (r *Upnp) Annotate(a infer.Annotator) {
	a.Describe(&r, "Whether UPnP is enabled. UPnP lets any LAN device open its own NAT hole, unrecorded.")
}

// Dmz asserts whether the DMZ host is enabled.
type Dmz struct{ toggle }

// Annotate describes the resource.
func (r *Dmz) Annotate(a infer.Annotator) {
	a.Describe(&r, "Whether the DMZ host is enabled. A DMZ host receives every unmatched inbound port.")
}

// RemoteAdmin asserts whether the router can be administered from the WAN side.
type RemoteAdmin struct{ toggle }

// Annotate describes the resource.
func (r *RemoteAdmin) Annotate(a infer.Annotator) {
	a.Describe(&r, "Whether the router can be administered from the WAN side.")
}
