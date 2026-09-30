package router

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// IngressTable is an inbound NAT table: a positional list of rules, unlike the
// single-field toggles.
type IngressTable struct {
	// Name is how the table reads in a message.
	Name   string
	target Target
}

// The inbound NAT tables.
var (
	PortForwarding = IngressTable{"port-forwarding", Target{"admin/nat", "vs"}}
	PortTriggering = IngressTable{"port-triggering", Target{"admin/nat", "pt"}}
)

// IngressTables is every inbound NAT table, for a caller asserting the whole surface.
var IngressTables = []IngressTable{PortForwarding, PortTriggering}

// IngressRule is one row of an inbound NAT table.
type IngressRule struct {
	// Index is the row's position, which is how the firmware addresses it.
	Index int
	// Summary flattens the row's fields for a log line or a drift message.
	Summary string
}

// IngressRules lists the rules in one table. Both tables have only ever been sampled
// empty on the reference router, so the row schema is unknown and rows are described
// generically rather than by field names that might not exist.
func (s *Session) IngressRules(ctx context.Context, table IngressTable) ([]IngressRule, error) {
	rows, err := s.loadRows(ctx, "load "+table.Name, table.target)
	if err != nil {
		return nil, err
	}
	rules := make([]IngressRule, len(rows))
	for i, row := range rows {
		rules[i] = IngressRule{Index: i, Summary: summarise(row)}
	}
	return rules, nil
}

// ClearIngressRules deletes every rule in a table and returns what it removed.
//
// Rows are addressed by position and the firmware ignores `key`, so the table is
// walked backwards: removing the last row cannot shift the index of any row still
// queued for removal, whereas removing the first would shift them all.
func (s *Session) ClearIngressRules(ctx context.Context, table IngressTable) ([]string, error) {
	rules, err := s.IngressRules(ctx, table)
	if err != nil {
		return nil, err
	}
	removed := make([]string, len(rules))
	for i := len(rules) - 1; i >= 0; i-- {
		what := fmt.Sprintf("remove %s rule at index %d", table.Name, rules[i].Index)
		_, err := s.call(ctx, what, table.target,
			url.Values{"operation": {"remove"}, "index": {strconv.Itoa(rules[i].Index)}})
		if err != nil {
			return removed, err
		}
		removed[i] = rules[i].Summary
	}
	return removed, nil
}
