package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

var (
	dhcpSetting    = Target{"admin/dhcps", "setting"}
	dhcpReserve    = Target{"admin/dhcps", "reservation"}
	deviceNameForm = Target{"admin/traffic", "dev_name"}
)

// NormalizeMAC accepts standard colon, dash, dotted, and twelve-digit MAC spellings.
func NormalizeMAC(mac string) (string, error) {
	if len(mac) == 12 {
		pairs := make([]string, 6)
		for i := range pairs {
			pairs[i] = mac[i*2 : i*2+2]
		}
		mac = strings.Join(pairs, ":")
	}
	parsed, err := net.ParseMAC(mac)
	if err != nil || len(parsed) != 6 {
		return "", fmt.Errorf("invalid MAC address: %q", mac)
	}
	return strings.ToUpper(strings.ReplaceAll(parsed.String(), ":", "-")), nil
}

// Reservation is one DHCP address reservation as the router stores it.
type Reservation struct {
	MAC string
	IP  string
	// DeviceName is the name carried on the reservation row. The name the router
	// displays for a device lives in a separate table; see SetDeviceName.
	DeviceName string
}

// Reservations lists every DHCP reservation.
func (s *Session) Reservations(ctx context.Context) ([]Reservation, error) {
	rows, err := s.loadRows(ctx, "load reservations", dhcpReserve)
	if err != nil {
		return nil, err
	}
	out := make([]Reservation, len(rows))
	for i, r := range rows {
		out[i] = Reservation{MAC: str(r, "mac"), IP: str(r, "ip"), DeviceName: str(r, "hostname")}
	}
	return out, nil
}

// FindReservation returns the reservation for mac and whether one exists. The mac
// must already be normalised.
func (s *Session) FindReservation(ctx context.Context, mac string) (Reservation, bool, error) {
	all, err := s.Reservations(ctx)
	if err != nil {
		return Reservation{}, false, err
	}
	for _, r := range all {
		if got, err := NormalizeMAC(r.MAC); err == nil && got == mac {
			return r, true, nil
		}
	}
	return Reservation{}, false, nil
}

// InsertReservation adds a reservation. `name` is the row's own hostname, which the
// router shows only for a MAC it has never seen; see SetDeviceName.
func (s *Session) InsertReservation(ctx context.Context, mac, ip, name string) error {
	row, err := json.Marshal(map[string]string{"mac": mac, "ip": ip, "hostname": name, "enable": "on"})
	if err != nil {
		return err
	}
	_, err = s.call(ctx, "insert reservation", dhcpReserve,
		url.Values{"operation": {"insert"}, "new": {string(row)}, "index": {"0"}})
	return err
}

// RemoveReservation deletes the reservation for mac, which must already be
// normalised. Deleting an absent reservation is not an error.
//
// The firmware addresses the row by position and ignores `key`, so removing with a
// fixed index deletes an unrelated device. The position is therefore resolved from a
// fresh listing on every call.
func (s *Session) RemoveReservation(ctx context.Context, mac string) error {
	all, err := s.Reservations(ctx)
	if err != nil {
		return err
	}
	for i, r := range all {
		if got, err := NormalizeMAC(r.MAC); err != nil || got != mac {
			continue
		}
		_, err := s.call(ctx, "remove reservation", dhcpReserve,
			url.Values{"operation": {"remove"}, "key": {mac}, "index": {strconv.Itoa(i)}})
		return err
	}
	return nil
}

// SetDeviceName sets the name the router displays for a device.
//
// This is a device-table write keyed by MAC, not a property of a reservation: it
// applies to any device, reserved or not, online or not, and outranks the hostname on
// a reservation row for every device the router has seen. An empty alias is accepted
// and then ignored, so callers must reject empty names rather than trust the reply.
func (s *Session) SetDeviceName(ctx context.Context, mac, name string) error {
	_, err := s.call(ctx, "set device name for "+mac, deviceNameForm,
		url.Values{"operation": {"write"}, "mac": {mac}, "alias": {name}})
	return err
}

// DHCP server field names in the `setting` record.
const (
	FieldPrimaryDNS   = "pri_dns"
	FieldSecondaryDNS = "snd_dns"
	FieldPoolStart    = "ipaddr_start"
	FieldPoolEnd      = "ipaddr_end"
)

// ReadDHCPServer reads the DHCP server record.
func (s *Session) ReadDHCPServer(ctx context.Context) (Form, error) {
	form, err := s.ReadForm(ctx, dhcpSetting)
	if err != nil {
		return nil, err
	}
	for _, field := range []string{FieldPrimaryDNS, FieldSecondaryDNS, FieldPoolStart, FieldPoolEnd, "gateway", "enable", "leasetime", "domain"} {
		if _, ok := form[field]; !ok {
			return nil, fmt.Errorf("DHCP settings missing %q", field)
		}
	}
	return form, nil
}

// WriteDHCPServer replaces the DHCP server record. Pass back every field from
// ReadDHCPServer, changed where needed, so lease time, gateway and domain survive.
func (s *Session) WriteDHCPServer(ctx context.Context, values Form) error {
	return s.WriteForm(ctx, dhcpSetting, values)
}
