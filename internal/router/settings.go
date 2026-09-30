package router

import (
	"context"
	"errors"
)

// Toggle names a capability the firmware models as a single `enable` field.
type Toggle struct {
	Target
}

// The toggles the provider exposes.
var (
	UPnP        = Toggle{Target{"admin/upnp", "enable"}}
	DMZ         = Toggle{Target{"admin/nat", "dmz"}}
	RemoteAdmin = Toggle{Target{"admin/administration", "remote"}}
)

const (
	enableField = "enable"
	on          = "on"
	off         = "off"
)

// ReadToggle reports whether the toggle is on.
func (s *Session) ReadToggle(ctx context.Context, t Toggle) (bool, error) {
	form, err := s.ReadForm(ctx, t.Target)
	if err != nil {
		return false, err
	}
	v, ok := form[enableField]
	if !ok {
		return false, errors.New("router did not report an `enable` field for " + t.Path)
	}
	if v != on && v != off {
		return false, errors.New("unknown toggle value")
	}
	return v == on, nil
}

// SetToggle turns a toggle on or off, leaving the rest of its record alone.
func (s *Session) SetToggle(ctx context.Context, t Toggle, enabled bool) error {
	v := off
	if enabled {
		v = on
	}
	return s.AssertForm(ctx, t.Target, Form{enableField: v})
}
