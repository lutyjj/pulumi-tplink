package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

var parentalControls = Target{"admin/avira_parental_control", "avira_pactrl"}

// newProfileID is the ownerId the firmware takes as "create a profile".
const newProfileID = "-1"

// Profile is one parental-controls profile: a named group of devices whose internet
// access the router can block while leaving their LAN traffic alone.
type Profile struct {
	// ID is the ownerId the router assigned. Empty for a profile not yet created.
	ID              string
	Name            string
	Devices         []string
	InternetBlocked bool

	// The remaining fields are not managed by the provider. They are carried from a
	// read into the next save so that an edit keeps them as the router holds them.
	age        int
	bedtime    json.RawMessage
	categories json.RawMessage
	websites   json.RawMessage
}

// rawOwner is a profile as getOwnerTotalData reports it. The firmware sends an empty
// list as `{}`, and ownerId as a string here but as a number from addOwnerInList.
type rawOwner struct {
	OwnerID         json.Number     `json:"ownerId"`
	Name            string          `json:"name"`
	Age             int             `json:"age"`
	InternetBlocked bool            `json:"internetBlocked"`
	ClientList      json.RawMessage `json:"clientList"`
	Bedtime         struct {
		Enable   bool            `json:"enable"`
		Everyday json.RawMessage `json:"everyday"`
	} `json:"bedtime"`
	FilterCategoriesList json.RawMessage `json:"filterCategoriesList"`
	FilterWebsiteList    json.RawMessage `json:"filterWebsiteList"`
}

// Profiles lists every parental-controls profile.
func (s *Session) Profiles(ctx context.Context) ([]Profile, error) {
	env, err := s.call(ctx, "read parental-controls profiles", parentalControls, url.Values{"operation": {"getOwnerTotalData"}})
	if err != nil {
		return nil, err
	}
	return decodeProfiles(env.Data)
}

func decodeProfiles(data json.RawMessage) ([]Profile, error) {
	var total struct {
		OwnerList json.RawMessage `json:"ownerList"`
	}
	if err := json.Unmarshal(data, &total); err != nil {
		return nil, fmt.Errorf("unreadable parental-controls profiles: %w", err)
	}
	var owners []rawOwner
	if err := decodeList(total.OwnerList, &owners); err != nil {
		return nil, fmt.Errorf("parental-controls profiles: %w", err)
	}
	out := make([]Profile, 0, len(owners))
	for _, o := range owners {
		var clients []struct {
			MAC string `json:"mac"`
		}
		if err := decodeList(o.ClientList, &clients); err != nil {
			return nil, fmt.Errorf("parental-controls profile %q devices: %w", o.Name, err)
		}
		devices := make([]string, 0, len(clients))
		for _, c := range clients {
			mac, err := NormalizeMAC(c.MAC)
			if err != nil {
				return nil, fmt.Errorf("parental-controls profile %q: %w", o.Name, err)
			}
			devices = append(devices, mac)
		}
		if o.OwnerID == "" {
			return nil, fmt.Errorf("parental-controls profile %q has no ownerId", o.Name)
		}
		// The write form takes only the everyday window; it is all the web UI sends.
		bedtime, err := json.Marshal(map[string]any{"enable": o.Bedtime.Enable, "everyday": o.Bedtime.Everyday})
		if err != nil {
			return nil, err
		}
		out = append(out, Profile{
			ID: o.OwnerID.String(), Name: o.Name, Devices: devices, InternetBlocked: o.InternetBlocked,
			age: o.Age, bedtime: bedtime,
			categories: listOrEmpty(o.FilterCategoriesList), websites: listOrEmpty(o.FilterWebsiteList),
		})
	}
	return out, nil
}

// decodeList reads a JSON array, accepting the firmware's `{}` for an empty one.
func decodeList(data json.RawMessage, into any) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil
	}
	if !strings.HasPrefix(trimmed, "[") {
		return fmt.Errorf("expected a list or an empty object")
	}
	return json.Unmarshal(data, into)
}

// listOrEmpty returns a list field as the write form expects it: a JSON array.
func listOrEmpty(data json.RawMessage) json.RawMessage {
	if strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
		return data
	}
	return json.RawMessage("[]")
}

// SaveProfile creates a profile when p.ID is empty and edits it otherwise, returning
// its ID. Saving records the internet-block flag but does not enforce a block: only
// SetProfileBlocked does, so callers follow a save with it.
func (s *Session) SaveProfile(ctx context.Context, p Profile) (string, error) {
	id := p.ID
	if id == "" {
		id = newProfileID
	}
	devices, err := json.Marshal(p.Devices)
	if err != nil {
		return "", err
	}
	body := url.Values{
		"operation":            {"addOwnerInList"},
		"ownerId":              {id},
		"name":                 {p.Name},
		"age":                  {strconv.Itoa(p.age)},
		"internetBlocked":      {strconv.FormatBool(p.InternetBlocked)},
		"allDeviceMac":         {string(devices)},
		"filterCategoriesList": {string(orEmptyList(p.categories))},
		"filterWebsiteList":    {string(orEmptyList(p.websites))},
		"bedtime":              {string(orDefaultBedtime(p.bedtime))},
	}
	env, err := s.call(ctx, fmt.Sprintf("save parental-controls profile %q", p.Name), parentalControls, body)
	if err != nil {
		return "", err
	}
	var saved struct {
		OwnerID json.Number `json:"ownerId"`
	}
	if err := json.Unmarshal(env.Data, &saved); err != nil || saved.OwnerID == "" {
		return "", fmt.Errorf("save parental-controls profile %q: the router returned no ownerId", p.Name)
	}
	return saved.OwnerID.String(), nil
}

// SetProfileBlocked blocks or restores internet access for every device in a profile.
// Their LAN traffic is unaffected.
func (s *Session) SetProfileBlocked(ctx context.Context, id string, blocked bool) error {
	_, err := s.call(ctx, fmt.Sprintf("set internet block on parental-controls profile %s", id), parentalControls, url.Values{
		"operation": {"internetBlock"}, "ownerId": {id}, "internetBlocked": {strconv.FormatBool(blocked)},
	})
	return err
}

// DeleteProfile removes a profile, which also lifts its internet block.
func (s *Session) DeleteProfile(ctx context.Context, id string) error {
	ids, err := json.Marshal([]string{id})
	if err != nil {
		return err
	}
	_, err = s.call(ctx, fmt.Sprintf("delete parental-controls profile %s", id), parentalControls, url.Values{
		"operation": {"delOwnerInList"}, "ownerList": {string(ids)},
	})
	return err
}

func orEmptyList(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return json.RawMessage("[]")
	}
	return data
}

// orDefaultBedtime is the web UI's default for a new profile: bedtime off, 21:00 to 07:00.
func orDefaultBedtime(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return json.RawMessage(`{"enable":false,"everyday":{"bedtimeBegin":1260,"bedtimeEnd":420}}`)
	}
	return data
}
