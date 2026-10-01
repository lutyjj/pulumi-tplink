package router

import (
	"context"
	"fmt"
)

// Toggle names a capability the firmware models as one on/off field.
type Toggle struct {
	Target
	field string
	// alone writes the field without echoing the record's other fields back, as
	// the web UI does for records whose remaining fields it never sends.
	alone bool
}

// The toggles the provider exposes.
var (
	UPnP         = Toggle{Target: Target{"admin/upnp", "enable"}, field: enableField}
	DMZ          = Toggle{Target: Target{"admin/nat", "dmz"}, field: enableField}
	RemoteAdmin  = Toggle{Target: Target{"admin/administration", "remote"}, field: enableField}
	EasyMesh     = Toggle{Target: Target{"admin/easymesh", "easymesh_enable"}, field: enableField, alone: true}
	MediaSharing = Toggle{Target: Target{"admin/folder_sharing", "media"}, field: "media_sharing", alone: true}
)

const (
	enableField = "enable"
	on          = "on"
	off         = "off"
)

func onOff(enabled bool) string {
	if enabled {
		return on
	}
	return off
}

// parseOnOff reads a field the firmware spells `on` or `off`, failing closed on anything else.
func parseOnOff(form Form, field string) (bool, error) {
	v, ok := form[field]
	if !ok {
		return false, fmt.Errorf("router did not report the %q field", field)
	}
	if v != on && v != off {
		return false, fmt.Errorf("unknown value for the %q field", field)
	}
	return v == on, nil
}

// ReadToggle reports whether the toggle is on.
func (s *Session) ReadToggle(ctx context.Context, t Toggle) (bool, error) {
	form, err := s.ReadForm(ctx, t.Target)
	if err != nil {
		return false, err
	}
	enabled, err := parseOnOff(form, t.field)
	if err != nil {
		return false, fmt.Errorf("%s?form=%s: %w", t.Path, t.Form, err)
	}
	return enabled, nil
}

// SetToggle turns a toggle on or off. A toggle that is not written alone leaves the
// rest of its record as the router holds it.
func (s *Session) SetToggle(ctx context.Context, t Toggle, enabled bool) error {
	values := Form{t.field: onOff(enabled)}
	if t.alone {
		return s.WriteForm(ctx, t.Target, values)
	}
	return s.AssertForm(ctx, t.Target, values)
}

var natALG = Target{"admin/nat", "alg"}

// ALG holds the NAT application-layer gateways. FTP, TFTP, H323, RTSP and SIP rewrite
// those protocols' payloads; PPTP, L2TP and IPsec pass VPN traffic through NAT.
type ALG struct {
	FTP, TFTP, H323, RTSP, SIP, PPTP, L2TP, IPsec bool
}

// fields maps each firmware field to its switch.
func (a *ALG) fields() map[string]*bool {
	return map[string]*bool{
		"ftp": &a.FTP, "tftp": &a.TFTP, "h323": &a.H323, "rtsp": &a.RTSP,
		"sip": &a.SIP, "pptp": &a.PPTP, "l2tp": &a.L2TP, "ipsec": &a.IPsec,
	}
}

// ReadALG reports every gateway. A missing or unknown field fails the read.
func (s *Session) ReadALG(ctx context.Context) (ALG, error) {
	form, err := s.ReadForm(ctx, natALG)
	if err != nil {
		return ALG{}, err
	}
	var a ALG
	for field, enabled := range a.fields() {
		if *enabled, err = parseOnOff(form, field); err != nil {
			return ALG{}, fmt.Errorf("NAT ALG record: %w", err)
		}
	}
	return a, nil
}

// SetALG sets every gateway. The web UI also writes all of them together.
func (s *Session) SetALG(ctx context.Context, a ALG) error {
	values := Form{}
	for field, enabled := range a.fields() {
		values[field] = onOff(*enabled)
	}
	return s.AssertForm(ctx, natALG, values)
}
