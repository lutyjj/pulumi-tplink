package provider

import (
	"context"

	"github.com/pulumi/pulumi-go-provider/infer"

	"github.com/lutyjj/pulumi-tplink/internal/router"
)

// DhcpServer manages the DHCP server record: the DNS servers handed to clients and
// the bounds of the dynamic address pool.
//
// Destroying it stops managing the record and leaves the last-applied values in
// place. Blanking them could break name resolution and addressing network-wide;
// change the inputs instead.
type DhcpServer struct{}

// Annotate describes the resource.
func (r *DhcpServer) Annotate(a infer.Annotator) {
	a.Describe(&r, "The router's DHCP server record: the DNS handed to clients and the dynamic pool bounds. Every other field (lease time, gateway, domain) is preserved.")
}

// DhcpServerArgs are the inputs of a DhcpServer.
type DhcpServerArgs struct {
	Primary   string  `pulumi:"primary"`
	Secondary *string `pulumi:"secondary,optional"`
	PoolStart *string `pulumi:"poolStart,optional"`
	PoolEnd   *string `pulumi:"poolEnd,optional"`
}

// Annotate describes the inputs.
func (a *DhcpServerArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Primary, "The primary DNS server handed to DHCP clients.")
	an.Describe(&a.Secondary, "The secondary DNS server. Omit for none: the router's secondary is cleared.")
	an.Describe(&a.PoolStart, "The first address of the dynamic pool. Omit to leave the router's value.")
	an.Describe(&a.PoolEnd, "The last address of the dynamic pool. Omit to leave the router's value.")
}

// DhcpServerState is the recorded state of a DhcpServer.
type DhcpServerState struct {
	DhcpServerArgs
}

// Check validates every address.
func (DhcpServer) Check(
	ctx context.Context, req infer.CheckRequest,
) (infer.CheckResponse[DhcpServerArgs], error) {
	args, failures, err := infer.DefaultCheck[DhcpServerArgs](ctx, req.NewInputs)
	if err != nil {
		return infer.CheckResponse[DhcpServerArgs]{}, err
	}
	primary := args.Primary
	if !req.NewInputs.Get("primary").IsComputed() {
		failures = append(failures, ipv4Failure("primary", &primary)...)
	}
	if !req.NewInputs.Get("secondary").IsComputed() {
		failures = append(failures, ipv4Failure("secondary", args.Secondary)...)
	}
	if !req.NewInputs.Get("poolStart").IsComputed() {
		failures = append(failures, ipv4Failure("poolStart", args.PoolStart)...)
	}
	if !req.NewInputs.Get("poolEnd").IsComputed() {
		failures = append(failures, ipv4Failure("poolEnd", args.PoolEnd)...)
	}
	return infer.CheckResponse[DhcpServerArgs]{Inputs: args, Failures: failures}, nil
}

func apply(ctx context.Context, args DhcpServerArgs) error {
	client, _ := connection(ctx)
	return client.Do(ctx, func(s *router.Session) error {
		current, err := s.ReadDHCPServer(ctx)
		if err != nil {
			return err
		}
		current[router.FieldPrimaryDNS] = args.Primary
		current[router.FieldSecondaryDNS] = deref(args.Secondary)
		if args.PoolStart != nil {
			current[router.FieldPoolStart] = *args.PoolStart
		}
		if args.PoolEnd != nil {
			current[router.FieldPoolEnd] = *args.PoolEnd
		}
		return s.WriteDHCPServer(ctx, current)
	})
}

// Create applies the inputs to the record.
func (DhcpServer) Create(
	ctx context.Context, req infer.CreateRequest[DhcpServerArgs],
) (infer.CreateResponse[DhcpServerState], error) {
	_, host := connection(ctx)
	out := infer.CreateResponse[DhcpServerState]{ID: host + "/dhcp-server", Output: DhcpServerState{req.Inputs}}
	if req.DryRun {
		return out, nil
	}
	return out, apply(ctx, req.Inputs)
}

// Update applies the new inputs to the record.
func (DhcpServer) Update(
	ctx context.Context, req infer.UpdateRequest[DhcpServerArgs, DhcpServerState],
) (infer.UpdateResponse[DhcpServerState], error) {
	out := infer.UpdateResponse[DhcpServerState]{Output: DhcpServerState{req.Inputs}}
	if req.DryRun {
		return out, nil
	}
	return out, apply(ctx, req.Inputs)
}

// Read reports the record as the router holds it.
func (DhcpServer) Read(
	ctx context.Context, req infer.ReadRequest[DhcpServerArgs, DhcpServerState],
) (infer.ReadResponse[DhcpServerArgs, DhcpServerState], error) {
	client, _ := connection(ctx)
	var current router.Form
	err := client.Do(ctx, func(s *router.Session) (err error) {
		current, err = s.ReadDHCPServer(ctx)
		return err
	})
	if err != nil {
		return infer.ReadResponse[DhcpServerArgs, DhcpServerState]{}, err
	}
	args := DhcpServerArgs{
		Primary:   current[router.FieldPrimaryDNS],
		Secondary: nonEmpty(current[router.FieldSecondaryDNS]),
	}
	// Pool bounds are reported only when managed; the rest are not ours.
	if req.State.PoolStart != nil || req.State.Primary == "" {
		args.PoolStart = nonEmpty(current[router.FieldPoolStart])
	}
	if req.State.PoolEnd != nil || req.State.Primary == "" {
		args.PoolEnd = nonEmpty(current[router.FieldPoolEnd])
	}
	return infer.ReadResponse[DhcpServerArgs, DhcpServerState]{ID: req.ID, Inputs: args, State: DhcpServerState{args}}, nil
}
