package provider

import (
	"fmt"
	"net/netip"

	p "github.com/pulumi/pulumi-go-provider"
)

// ipv4Failure validates an optional IPv4 address property.
func ipv4Failure(property string, value *string) []p.CheckFailure {
	if value == nil {
		return nil
	}
	if addr, err := netip.ParseAddr(*value); err != nil || !addr.Is4() {
		return []p.CheckFailure{{Property: property, Reason: fmt.Sprintf("invalid IPv4 address: %q", *value)}}
	}
	return nil
}
