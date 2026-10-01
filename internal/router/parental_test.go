package router

import (
	"encoding/json"
	"testing"
)

// The firmware spells an empty list `{}` and ownerId as a string; both must decode.
func TestDecodeProfiles(t *testing.T) {
	empty, err := decodeProfiles(json.RawMessage(`{"ownerList":{}}`))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty list: %v %v", empty, err)
	}
	got, err := decodeProfiles(json.RawMessage(`{"ownerList":[{"ownerId":"2","name":"iot","age":0,"internetBlocked":true,
		"clientList":[{"mac":"aa-bb-cc-dd-ee-01"}],"filterCategoriesList":{},"filterWebsiteList":{},
		"bedtime":{"enable":false,"mode":"everyday","everyday":{"bedtimeBegin":1260,"bedtimeEnd":420}}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "2" || !got[0].InternetBlocked || got[0].Devices[0] != "AA-BB-CC-DD-EE-01" {
		t.Fatalf("unexpected profile: %+v", got)
	}
	if string(got[0].categories) != "[]" {
		t.Fatalf("empty filter list must be sent as [], got %s", got[0].categories)
	}
	if _, err := decodeProfiles(json.RawMessage(`{"ownerList":"bad"}`)); err == nil {
		t.Fatal("an unrecognised list shape must fail, not read as empty")
	}
}
