package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"

	"github.com/lutyjj/pulumi-tplink/internal/router"
)

// DhcpReservation pins an IP address to a device by MAC, and sets the name the
// router displays for that device.
//
// The name is a write to a separate device table keyed by MAC rather than a field of
// the reservation, but it is applied in the same session. Deleting the resource drops
// the reservation and leaves the name in place: the firmware has no way to unset one.
type DhcpReservation struct{}

// Annotate describes the resource.
func (r *DhcpReservation) Annotate(a infer.Annotator) {
	a.Describe(&r, "A DHCP address reservation, plus the name the router displays for the device.")
}

// ReservationArgs are the inputs of a DhcpReservation.
type ReservationArgs struct {
	MAC        string  `pulumi:"mac"`
	IP         string  `pulumi:"ip"`
	DeviceName *string `pulumi:"deviceName,optional"`
}

// Annotate describes the inputs.
func (a *ReservationArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.MAC, "The device's MAC address, in any spelling. Normalised to AA-BB-CC-DD-EE-FF. Changing it replaces the reservation.")
	an.Describe(&a.IP, "The IPv4 address to reserve for the device.")
	an.Describe(&a.DeviceName, "The name the router displays for the device. Omit to leave whatever name the router currently shows.")
}

// ReservationState is the recorded state of a DhcpReservation.
type ReservationState struct {
	ReservationArgs
	Enabled bool `pulumi:"enabled"`
}

// Annotate describes the observed enable state.
func (s *ReservationState) Annotate(a infer.Annotator) {
	a.Describe(&s.Enabled, "Whether the router enables this reservation. Apply always enables it; refresh detects reservations disabled outside Pulumi.")
}

// Check normalises the MAC and validates the rest.
func (DhcpReservation) Check(
	ctx context.Context, req infer.CheckRequest,
) (infer.CheckResponse[ReservationArgs], error) {
	args, failures, err := infer.DefaultCheck[ReservationArgs](ctx, req.NewInputs)
	if err != nil {
		return infer.CheckResponse[ReservationArgs]{}, err
	}
	if mac, err := router.NormalizeMAC(args.MAC); !req.NewInputs.Get("mac").IsComputed() && err != nil {
		failures = append(failures, p.CheckFailure{Property: "mac", Reason: err.Error()})
	} else if !req.NewInputs.Get("mac").IsComputed() {
		args.MAC = mac
	}
	ip := args.IP
	if !req.NewInputs.Get("ip").IsComputed() {
		failures = append(failures, ipv4Failure("ip", &ip)...)
	}
	// The router answers success to an empty alias and keeps the old name, so an
	// empty value would deploy clean and change nothing.
	if args.DeviceName != nil && !req.NewInputs.Get("deviceName").IsComputed() && strings.TrimSpace(*args.DeviceName) == "" {
		failures = append(failures, p.CheckFailure{
			Property: "deviceName",
			Reason:   "must not be empty: the router silently ignores an empty name; omit the property to leave the current name alone",
		})
	}
	return infer.CheckResponse[ReservationArgs]{Inputs: args, Failures: failures}, nil
}

// Diff replaces on a new MAC and updates in place for everything else.
func (DhcpReservation) Diff(
	_ context.Context, req infer.DiffRequest[ReservationArgs, ReservationState],
) (infer.DiffResponse, error) {
	diff := map[string]p.PropertyDiff{}
	if req.Inputs.MAC != req.State.MAC {
		diff["mac"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if req.Inputs.IP != req.State.IP {
		diff["ip"] = p.PropertyDiff{Kind: p.Update}
	}
	if !req.State.Enabled {
		diff["enabled"] = p.PropertyDiff{Kind: p.Update}
	}
	// An omitted name is not managed, so it never drifts.
	if req.Inputs.DeviceName != nil && deref(req.Inputs.DeviceName) != deref(req.State.DeviceName) {
		diff["deviceName"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{
		HasChanges:          len(diff) > 0,
		DetailedDiff:        diff,
		DeleteBeforeReplace: true,
	}, nil
}

// Create adopts a matching reservation, or makes the router agree with the inputs.
func (DhcpReservation) Create(
	ctx context.Context, req infer.CreateRequest[ReservationArgs],
) (infer.CreateResponse[ReservationState], error) {
	client, host := connection(ctx)
	id := host + "/" + req.Inputs.MAC
	state := ReservationState{ReservationArgs: req.Inputs, Enabled: true}
	if req.DryRun {
		return infer.CreateResponse[ReservationState]{ID: id, Output: state}, nil
	}
	err := client.Do(ctx, func(s *router.Session) error {
		return converge(ctx, s, req.Inputs)
	})
	return infer.CreateResponse[ReservationState]{ID: id, Output: state}, err
}

// Update moves the reservation to a new address and/or renames the device.
func (DhcpReservation) Update(
	ctx context.Context, req infer.UpdateRequest[ReservationArgs, ReservationState],
) (infer.UpdateResponse[ReservationState], error) {
	state := ReservationState{ReservationArgs: req.Inputs, Enabled: true}
	if req.DryRun {
		return infer.UpdateResponse[ReservationState]{Output: state}, nil
	}
	client, _ := connection(ctx)
	err := client.Do(ctx, func(s *router.Session) error {
		return converge(ctx, s, req.Inputs)
	})
	return infer.UpdateResponse[ReservationState]{Output: state}, err
}

// converge makes the router hold args. It is idempotent, which is what lets Create
// adopt a reservation that already exists instead of inserting a duplicate.
func converge(ctx context.Context, s *router.Session, args ReservationArgs) error {
	existing, found, err := s.FindReservation(ctx, args.MAC)
	if err != nil {
		return err
	}
	name := deref(args.DeviceName)
	// Name first, reservation last. Creating takes two writes and only the row is
	// visible to Pulumi as "the resource exists", so the row is the commit point.
	// Were it written first, a failure in between would leave a row Pulumi has no
	// record of. The name write has no such hazard: it is idempotent.
	if name != "" {
		if err := s.SetDeviceName(ctx, args.MAC, name); err != nil {
			return err
		}
	}
	if found && existing.IP == args.IP && existing.Enabled {
		return nil
	}
	if found {
		// The addressing of `operation=update` is unverified. Use the verified
		// remove and insert operations, resolving positions from fresh reads.
		if err := s.RemoveReservation(ctx, args.MAC); err != nil {
			return err
		}
	}
	rowName := name
	if rowName == "" {
		rowName = existing.DeviceName
	}
	if err := s.InsertReservation(ctx, router.Reservation{MAC: args.MAC, IP: args.IP, DeviceName: rowName, Enabled: true}); err != nil {
		// Restore the old row if a remove+insert update fails. Retrying will reconcile it.
		if found {
			rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer cancel()
			if restoreErr := s.InsertReservation(rctx, existing); restoreErr != nil {
				return fmt.Errorf("insert failed (%w); rollback also failed: %v", err, restoreErr)
			}
		}
		return err
	}
	// The fresh row carries its own hostname, which is what the router shows for a
	// device it has never seen, so assert the name once more.
	if found && name != "" {
		return s.SetDeviceName(ctx, args.MAC, name)
	}
	return nil
}

// Read reports the router's current view, so drift shows up on refresh.
func (DhcpReservation) Read(
	ctx context.Context, req infer.ReadRequest[ReservationArgs, ReservationState],
) (infer.ReadResponse[ReservationArgs, ReservationState], error) {
	rawMAC := req.State.MAC
	importing := rawMAC == ""
	if importing {
		_, rawMAC, _ = strings.Cut(req.ID, "/")
		if rawMAC == "" {
			rawMAC = req.ID
		}
	}
	mac, err := router.NormalizeMAC(rawMAC)
	if err != nil {
		return infer.ReadResponse[ReservationArgs, ReservationState]{}, err
	}
	client, _ := connection(ctx)
	var found router.Reservation
	var ok bool
	err = client.Do(ctx, func(s *router.Session) (err error) {
		found, ok, err = s.FindReservation(ctx, mac)
		return err
	})
	if err != nil || !ok {
		// An empty ID tells Pulumi the reservation was deleted out of band.
		return infer.ReadResponse[ReservationArgs, ReservationState]{}, err
	}
	args := ReservationArgs{MAC: mac, IP: found.IP, DeviceName: req.State.DeviceName}
	// Report the router's name only when this resource manages one: otherwise it is
	// someone else's value and would drift against nothing.
	if req.State.DeviceName != nil || importing {
		args.DeviceName = &found.DeviceName
	}
	_, host := connection(ctx)
	return infer.ReadResponse[ReservationArgs, ReservationState]{
		ID: host + "/" + mac, Inputs: args, State: ReservationState{ReservationArgs: args, Enabled: found.Enabled},
	}, nil
}

// Delete removes the reservation. The device's displayed name stays.
func (DhcpReservation) Delete(
	ctx context.Context, req infer.DeleteRequest[ReservationState],
) (infer.DeleteResponse, error) {
	mac, err := router.NormalizeMAC(req.State.MAC)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	client, _ := connection(ctx)
	return infer.DeleteResponse{}, client.Do(ctx, func(s *router.Session) error {
		return s.RemoveReservation(ctx, mac)
	})
}
