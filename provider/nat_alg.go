package provider

import (
	"context"

	"github.com/pulumi/pulumi-go-provider/infer"

	"github.com/lutyjj/pulumi-tplink/internal/router"
)

// NatAlg manages the NAT application-layer gateways as one record.
//
// Destroying it stops asserting the gateways and leaves the router as it is.
type NatAlg struct{}

// Annotate describes the resource.
func (r *NatAlg) Annotate(a infer.Annotator) {
	a.Describe(&r, "The NAT application-layer gateways (ALGs). Every gateway is declared, as the web UI writes them together.")
}

// NatAlgArgs are the inputs of a NatAlg.
type NatAlgArgs struct {
	Ftp   bool `pulumi:"ftp"`
	Tftp  bool `pulumi:"tftp"`
	H323  bool `pulumi:"h323"`
	Rtsp  bool `pulumi:"rtsp"`
	Sip   bool `pulumi:"sip"`
	Pptp  bool `pulumi:"pptp"`
	L2tp  bool `pulumi:"l2tp"`
	Ipsec bool `pulumi:"ipsec"`
}

// Annotate describes the inputs.
func (a *NatAlgArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Ftp, "Whether the FTP ALG rewrites FTP data connections.")
	an.Describe(&a.Tftp, "Whether the TFTP ALG tracks TFTP transfers.")
	an.Describe(&a.H323, "Whether the H.323 ALG rewrites H.323 call signalling.")
	an.Describe(&a.Rtsp, "Whether the RTSP ALG rewrites RTSP streaming sessions.")
	an.Describe(&a.Sip, "Whether the SIP ALG rewrites SIP call signalling.")
	an.Describe(&a.Pptp, "Whether PPTP VPN connections pass through NAT.")
	an.Describe(&a.L2tp, "Whether L2TP VPN connections pass through NAT.")
	an.Describe(&a.Ipsec, "Whether IPsec VPN connections pass through NAT.")
}

// NatAlgState is the recorded state of a NatAlg.
type NatAlgState struct {
	NatAlgArgs
}

func (a NatAlgArgs) alg() router.ALG {
	return router.ALG{
		FTP: a.Ftp, TFTP: a.Tftp, H323: a.H323, RTSP: a.Rtsp,
		SIP: a.Sip, PPTP: a.Pptp, L2TP: a.L2tp, IPsec: a.Ipsec,
	}
}

func natAlgArgs(a router.ALG) NatAlgArgs {
	return NatAlgArgs{
		Ftp: a.FTP, Tftp: a.TFTP, H323: a.H323, Rtsp: a.RTSP,
		Sip: a.SIP, Pptp: a.PPTP, L2tp: a.L2TP, Ipsec: a.IPsec,
	}
}

func setNatAlg(ctx context.Context, args NatAlgArgs) error {
	client, _ := connection(ctx)
	return client.Do(ctx, func(s *router.Session) error { return s.SetALG(ctx, args.alg()) })
}

// Create asserts the gateways.
func (NatAlg) Create(
	ctx context.Context, req infer.CreateRequest[NatAlgArgs],
) (infer.CreateResponse[NatAlgState], error) {
	_, host := connection(ctx)
	out := infer.CreateResponse[NatAlgState]{ID: host + "/nat-alg", Output: NatAlgState{req.Inputs}}
	if req.DryRun {
		return out, nil
	}
	return out, setNatAlg(ctx, req.Inputs)
}

// Update asserts the new gateways.
func (NatAlg) Update(
	ctx context.Context, req infer.UpdateRequest[NatAlgArgs, NatAlgState],
) (infer.UpdateResponse[NatAlgState], error) {
	out := infer.UpdateResponse[NatAlgState]{Output: NatAlgState{req.Inputs}}
	if req.DryRun {
		return out, nil
	}
	return out, setNatAlg(ctx, req.Inputs)
}

// Read reports the gateways the router holds.
func (NatAlg) Read(
	ctx context.Context, req infer.ReadRequest[NatAlgArgs, NatAlgState],
) (infer.ReadResponse[NatAlgArgs, NatAlgState], error) {
	client, _ := connection(ctx)
	var current router.ALG
	err := client.Do(ctx, func(s *router.Session) (err error) {
		current, err = s.ReadALG(ctx)
		return err
	})
	if err != nil {
		return infer.ReadResponse[NatAlgArgs, NatAlgState]{}, err
	}
	args := natAlgArgs(current)
	return infer.ReadResponse[NatAlgArgs, NatAlgState]{ID: req.ID, Inputs: args, State: NatAlgState{args}}, nil
}

// Delete stops asserting the gateways and leaves the router as it is.
func (NatAlg) Delete(context.Context, infer.DeleteRequest[NatAlgState]) (infer.DeleteResponse, error) {
	return infer.DeleteResponse{}, nil
}
