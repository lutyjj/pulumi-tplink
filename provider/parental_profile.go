package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"

	"github.com/lutyjj/pulumi-tplink/internal/router"
)

// ParentalControlProfile is a parental-controls profile: a named group of devices
// whose internet access the router can block. Blocking leaves LAN traffic alone, so
// blocked devices stay reachable from the local network.
//
// The profile's content filters and bedtime schedule are not managed. Creating a
// profile leaves them off; editing one keeps them as the router holds them.
type ParentalControlProfile struct{}

// Annotate describes the resource.
func (r *ParentalControlProfile) Annotate(a infer.Annotator) {
	a.Describe(&r, "A parental-controls profile that groups devices and can block their internet access while leaving LAN traffic alone.")
}

// ProfileArgs are the inputs of a ParentalControlProfile.
type ProfileArgs struct {
	Name            string   `pulumi:"name"`
	Devices         []string `pulumi:"devices"`
	InternetBlocked bool     `pulumi:"internetBlocked"`
}

// Annotate describes the inputs.
func (a *ProfileArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "The profile name. A profile with this name that already exists is adopted on create.")
	an.Describe(&a.Devices, "MAC addresses of the profile's devices, in any spelling. Normalised to AA-BB-CC-DD-EE-FF and sorted.")
	an.Describe(&a.InternetBlocked, "Whether the profile's devices are cut off from the internet. Their LAN traffic is unaffected.")
}

// ProfileState is the recorded state of a ParentalControlProfile.
type ProfileState struct {
	ProfileArgs
}

// Check normalises, sorts and deduplicates the device MACs.
func (ParentalControlProfile) Check(
	ctx context.Context, req infer.CheckRequest,
) (infer.CheckResponse[ProfileArgs], error) {
	args, failures, err := infer.DefaultCheck[ProfileArgs](ctx, req.NewInputs)
	if err != nil {
		return infer.CheckResponse[ProfileArgs]{}, err
	}
	if !req.NewInputs.Get("name").IsComputed() && strings.TrimSpace(args.Name) == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "must not be empty"})
	}
	if req.NewInputs.Get("devices").IsComputed() {
		return infer.CheckResponse[ProfileArgs]{Inputs: args, Failures: failures}, nil
	}
	devices := make([]string, 0, len(args.Devices))
	for i, raw := range args.Devices {
		mac, err := router.NormalizeMAC(raw)
		if err != nil {
			failures = append(failures, p.CheckFailure{Property: fmt.Sprintf("devices[%d]", i), Reason: err.Error()})
			continue
		}
		if slices.Contains(devices, mac) {
			failures = append(failures, p.CheckFailure{Property: fmt.Sprintf("devices[%d]", i), Reason: "duplicate device " + mac})
			continue
		}
		devices = append(devices, mac)
	}
	slices.Sort(devices)
	args.Devices = devices
	return infer.CheckResponse[ProfileArgs]{Inputs: args, Failures: failures}, nil
}

// Diff updates every field in place: the router edits a profile under its ownerId.
func (ParentalControlProfile) Diff(
	_ context.Context, req infer.DiffRequest[ProfileArgs, ProfileState],
) (infer.DiffResponse, error) {
	diff := map[string]p.PropertyDiff{}
	if req.Inputs.Name != req.State.Name {
		diff["name"] = p.PropertyDiff{Kind: p.Update}
	}
	if !slices.Equal(req.Inputs.Devices, req.State.Devices) {
		diff["devices"] = p.PropertyDiff{Kind: p.Update}
	}
	if req.Inputs.InternetBlocked != req.State.InternetBlocked {
		diff["internetBlocked"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(diff) > 0, DetailedDiff: diff}, nil
}

// Create adopts a profile with the same name, or creates one.
func (ParentalControlProfile) Create(
	ctx context.Context, req infer.CreateRequest[ProfileArgs],
) (infer.CreateResponse[ProfileState], error) {
	client, host := connection(ctx)
	state := ProfileState{ProfileArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[ProfileState]{Output: state}, nil
	}
	var id string
	err := client.Do(ctx, func(s *router.Session) error {
		profiles, err := s.Profiles(ctx)
		if err != nil {
			return err
		}
		profile := router.Profile{}
		if i := slices.IndexFunc(profiles, func(p router.Profile) bool { return p.Name == req.Inputs.Name }); i >= 0 {
			profile = profiles[i]
		}
		id, err = applyProfile(ctx, s, profile, req.Inputs)
		return err
	})
	return infer.CreateResponse[ProfileState]{ID: profileID(host, id), Output: state}, err
}

// Update edits the profile, keeping its unmanaged settings.
func (ParentalControlProfile) Update(
	ctx context.Context, req infer.UpdateRequest[ProfileArgs, ProfileState],
) (infer.UpdateResponse[ProfileState], error) {
	state := ProfileState{ProfileArgs: req.Inputs}
	if req.DryRun {
		return infer.UpdateResponse[ProfileState]{Output: state}, nil
	}
	client, _ := connection(ctx)
	ownerID := ownerIDOf(req.ID)
	err := client.Do(ctx, func(s *router.Session) error {
		profile, ok, err := findProfile(ctx, s, ownerID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("parental-controls profile %s no longer exists; refresh to recreate it", ownerID)
		}
		_, err = applyProfile(ctx, s, profile, req.Inputs)
		return err
	})
	return infer.UpdateResponse[ProfileState]{Output: state}, err
}

// applyProfile saves the profile and then enforces its internet block. Saving only
// records the block flag; the firmware enforces it through a separate operation.
func applyProfile(ctx context.Context, s *router.Session, profile router.Profile, args ProfileArgs) (string, error) {
	profile.Name = args.Name
	profile.Devices = args.Devices
	profile.InternetBlocked = args.InternetBlocked
	id, err := s.SaveProfile(ctx, profile)
	if err != nil {
		return "", err
	}
	return id, s.SetProfileBlocked(ctx, id, args.InternetBlocked)
}

// Read reports the router's current view, so drift shows up on refresh.
func (ParentalControlProfile) Read(
	ctx context.Context, req infer.ReadRequest[ProfileArgs, ProfileState],
) (infer.ReadResponse[ProfileArgs, ProfileState], error) {
	client, host := connection(ctx)
	ownerID := ownerIDOf(req.ID)
	var profile router.Profile
	var ok bool
	err := client.Do(ctx, func(s *router.Session) (err error) {
		profile, ok, err = findProfile(ctx, s, ownerID)
		return err
	})
	if err != nil || !ok {
		// An empty ID tells Pulumi the profile was deleted out of band.
		return infer.ReadResponse[ProfileArgs, ProfileState]{}, err
	}
	devices := slices.Clone(profile.Devices)
	slices.Sort(devices)
	args := ProfileArgs{Name: profile.Name, Devices: devices, InternetBlocked: profile.InternetBlocked}
	return infer.ReadResponse[ProfileArgs, ProfileState]{
		ID: profileID(host, profile.ID), Inputs: args, State: ProfileState{ProfileArgs: args},
	}, nil
}

// Delete removes the profile, which lifts its internet block.
func (ParentalControlProfile) Delete(
	ctx context.Context, req infer.DeleteRequest[ProfileState],
) (infer.DeleteResponse, error) {
	client, _ := connection(ctx)
	ownerID := ownerIDOf(req.ID)
	return infer.DeleteResponse{}, client.Do(ctx, func(s *router.Session) error {
		_, ok, err := findProfile(ctx, s, ownerID)
		if err != nil || !ok {
			return err
		}
		return s.DeleteProfile(ctx, ownerID)
	})
}

func findProfile(ctx context.Context, s *router.Session, ownerID string) (router.Profile, bool, error) {
	profiles, err := s.Profiles(ctx)
	if err != nil {
		return router.Profile{}, false, err
	}
	i := slices.IndexFunc(profiles, func(p router.Profile) bool { return p.ID == ownerID })
	if i < 0 {
		return router.Profile{}, false, nil
	}
	return profiles[i], true, nil
}

// profileID scopes the router's ownerId to the router, as `host/ownerId`.
func profileID(host, ownerID string) string { return host + "/parental-profile/" + ownerID }

// ownerIDOf accepts a full resource ID or a bare ownerId, as given to import.
func ownerIDOf(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}
