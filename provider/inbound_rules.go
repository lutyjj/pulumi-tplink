package provider

import (
	"context"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"

	"github.com/lutyjj/pulumi-tplink/internal/router"
)

// InboundRules asserts that the router holds no inbound NAT rules: neither port
// forwards (virtual servers) nor port triggers.
//
// It is the list-shaped sibling of the toggles. A NAT table is a positional list, so
// asserting it means enumerating and removing rows rather than writing a value. Both
// tables belong to one resource because they are one invariant: no inbound path
// exists that nobody declared.
//
// Removing a rule on apply is the point rather than a side effect. A forward nobody
// declared is what this exists to catch, whether it arrived by hand, from a factory
// reset restoring defaults, or from another LAN user. The preview names every rule
// before anything is removed.
//
// Declaring a rule is not something this resource can express, so it takes no inputs:
// declaring it is the assertion.
type InboundRules struct{}

// Annotate describes the resource.
func (r *InboundRules) Annotate(a infer.Annotator) {
	a.Describe(&r, "Asserts that the router holds no port forwards and no port triggers, removing any it finds.")
}

// InboundRulesArgs has no inputs: declaring the resource is the assertion.
type InboundRulesArgs struct{}

// InboundRulesState records the currently observed rules and the last removal.
type InboundRulesState struct {
	Removed []string `pulumi:"removed"`
	Rules   []string `pulumi:"rules"`
}

// Annotate describes the outputs.
func (s *InboundRulesState) Annotate(a infer.Annotator) {
	a.Describe(&s.Rules, "Rules currently present, as `table: fields`. Empty after an apply.")
	a.Describe(&s.Removed, "The rules removed by the last apply, as `table: fields`.")
}

// observe lists every rule in both tables, labelled with its table.
func observe(ctx context.Context) ([]string, error) {
	client, _ := connection(ctx)
	var found []string
	err := client.Do(ctx, func(s *router.Session) error {
		for _, table := range router.IngressTables {
			rules, err := s.IngressRules(ctx, table)
			if err != nil {
				return err
			}
			for _, r := range rules {
				found = append(found, table.Name+": "+r.Summary)
			}
		}
		return nil
	})
	return found, err
}

func clearAll(ctx context.Context) ([]string, error) {
	client, _ := connection(ctx)
	removed := []string{}
	err := client.Do(ctx, func(s *router.Session) error {
		for _, table := range router.IngressTables {
			gone, err := s.ClearIngressRules(ctx, table)
			for _, summary := range gone {
				removed = append(removed, table.Name+": "+summary)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	return removed, err
}

// Diff reads the router rather than comparing stored state. It has to: preview and
// up do not refresh, so a pure comparison would set the empty list this resource last
// wrote against itself and report converged whatever the router had grown since.
// Detecting exactly that is the point, so the check runs on every preview, and an
// unreachable router fails loudly instead of reporting a clean diff it has not earned.
func (InboundRules) Diff(
	ctx context.Context, _ infer.DiffRequest[InboundRulesArgs, InboundRulesState],
) (infer.DiffResponse, error) {
	found, err := observe(ctx)
	if err != nil {
		return infer.DiffResponse{}, err
	}
	for _, rule := range found {
		p.GetLogger(ctx).Warningf("inbound rule present, will be removed: %s", rule)
	}
	diff := map[string]p.PropertyDiff{}
	if len(found) > 0 {
		diff["removed"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(found) > 0, DetailedDiff: diff}, nil
}

// Create removes every inbound rule.
func (InboundRules) Create(
	ctx context.Context, req infer.CreateRequest[InboundRulesArgs],
) (infer.CreateResponse[InboundRulesState], error) {
	_, host := connection(ctx)
	id := host + "/inbound-rules"
	if req.DryRun {
		return infer.CreateResponse[InboundRulesState]{ID: id, Output: InboundRulesState{Removed: []string{}, Rules: []string{}}}, nil
	}
	removed, err := clearAll(ctx)
	return infer.CreateResponse[InboundRulesState]{ID: id, Output: InboundRulesState{Removed: removed, Rules: []string{}}}, err
}

// Update removes every inbound rule.
func (InboundRules) Update(
	ctx context.Context, req infer.UpdateRequest[InboundRulesArgs, InboundRulesState],
) (infer.UpdateResponse[InboundRulesState], error) {
	if req.DryRun {
		return infer.UpdateResponse[InboundRulesState]{Output: req.State}, nil
	}
	removed, err := clearAll(ctx)
	return infer.UpdateResponse[InboundRulesState]{Output: InboundRulesState{Removed: removed, Rules: []string{}}}, err
}

// Read reports the rules the router holds right now.
func (InboundRules) Read(
	ctx context.Context, req infer.ReadRequest[InboundRulesArgs, InboundRulesState],
) (infer.ReadResponse[InboundRulesArgs, InboundRulesState], error) {
	found, err := observe(ctx)
	if err != nil {
		return infer.ReadResponse[InboundRulesArgs, InboundRulesState]{}, err
	}
	if found == nil {
		found = []string{}
	}
	return infer.ReadResponse[InboundRulesArgs, InboundRulesState]{
		ID: req.ID, Inputs: InboundRulesArgs{}, State: InboundRulesState{Removed: req.State.Removed, Rules: found},
	}, nil
}

// Delete stops asserting the invariant. It deliberately does not restore what it
// removed: nothing records those rules, and reopening a NAT hole on destroy would be
// the wrong default anyway.
func (InboundRules) Delete(context.Context, infer.DeleteRequest[InboundRulesState]) (infer.DeleteResponse, error) {
	return infer.DeleteResponse{}, nil
}
