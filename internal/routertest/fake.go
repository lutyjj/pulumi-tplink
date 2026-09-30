// Package routertest is an in-process fake of the router's web API, faithful to the
// firmware behaviours the provider has to work around: positional deletes that ignore
// `key`, an alias write that silently ignores an empty name, and a single admin slot.
package routertest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	// Password is the admin password the fake accepts.
	Password = "correct horse"
	keyBits  = 1024
)

// Reservation is one stored row.
type Reservation struct {
	MAC, IP, Hostname string
	Enabled           bool
}

// Fake is a running fake router. Its exported fields may be read or seeded under
// Lock while no request is in flight.
type Fake struct {
	// Host is the address to give the client, `127.0.0.1:port`.
	Host string

	mu sync.Mutex
	// Reservations is the reservation table, in row order.
	Reservations []Reservation
	// Names is the device-name table, keyed by normalised MAC.
	Names map[string]string
	// Forms holds settings records keyed by `path?form=name`.
	Forms map[string]map[string]string
	// Tables holds NAT tables keyed by `path?form=name`.
	Tables map[string][]map[string]string
	// Logins counts successful logins; MaxActive is the most sessions ever open at once.
	Logins, MaxActive int
	// FailNext rejects matching operations before they take effect.
	FailNext map[string]int

	key    *rsa.PrivateKey
	active map[string]bool
	srv    *httptest.Server
}

// New starts a fake router and stops it when the test ends.
func New(t testing.TB) *Fake {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		t.Fatal(err)
	}
	f := &Fake{
		Names: map[string]string{},
		Forms: map[string]map[string]string{
			"admin/dhcps?form=setting": {
				"enable": "on", "leasetime": "120", "pri_dns": "192.0.2.1", "snd_dns": "",
				"gateway": "192.0.2.1", "ipaddr_start": "192.0.2.100", "ipaddr_end": "192.0.2.199", "domain": "lan",
			},
			"admin/upnp?form=enable":           {"enable": "on"},
			"admin/nat?form=dmz":               {"enable": "off", "ipaddr": "0.0.0.0"},
			"admin/administration?form=remote": {"enable": "off", "port": "443"},
		},
		Tables: map[string][]map[string]string{
			"admin/nat?form=vs": {},
			"admin/nat?form=pt": {},
		},
		FailNext: map[string]int{},
		key:      key,
		active:   map[string]bool{},
	}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	f.Host = strings.TrimPrefix(f.srv.URL, "https://")
	t.Cleanup(f.srv.Close)
	return f
}

// Lock guards the exported state for a test that inspects or seeds it.
func (f *Fake) Lock()   { f.mu.Lock() }
func (f *Fake) Unlock() { f.mu.Unlock() }

// Snapshot returns a copy of the reservation table.
func (f *Fake) Snapshot() []Reservation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Reservation(nil), f.Reservations...)
}

func reply(w http.ResponseWriter, success bool, data any) {
	out := map[string]any{"success": success}
	if success {
		out["data"] = data
	} else {
		out["errorcode"] = data
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/cgi-bin/luci/;stok=")
	if !ok {
		http.NotFound(w, r)
		return
	}
	stok, path, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	if strings.HasPrefix(rest, "/login") {
		f.login(w, r)
		return
	}
	if !f.active[stok] {
		reply(w, false, "timeout")
		return
	}
	target := path + "?form=" + r.URL.Query().Get("form")
	op := r.PostForm.Get("operation")
	failKey := target + "/" + op
	if f.FailNext[failKey] > 0 {
		f.FailNext[failKey]--
		reply(w, false, "injected_failure")
		return
	}
	switch target {
	case "admin/system?form=logout":
		delete(f.active, stok)
		reply(w, true, map[string]any{})
	case "admin/dhcps?form=reservation":
		f.reservations(w, op, r.PostForm)
	case "admin/traffic?form=dev_name":
		f.deviceName(w, r.PostForm)
	default:
		f.generic(w, target, op, r.PostForm)
	}
}

func (f *Fake) login(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("form") {
	case "keys":
		n := hex.EncodeToString(f.key.N.Bytes())
		e := strconv.FormatInt(int64(f.key.E), 16)
		reply(w, true, map[string]any{"password": []string{n, e}})
	case "login":
		ct, err := hex.DecodeString(r.PostForm.Get("password"))
		if err != nil {
			reply(w, false, "bad_password_encoding")
			return
		}
		pw, err := rsa.DecryptPKCS1v15(rand.Reader, f.key, ct) //nolint:staticcheck // The fake implements the firmware protocol, not a public decryption oracle.
		if err != nil || string(pw) != Password {
			reply(w, false, "login_error")
			return
		}
		if len(f.active) > 0 {
			// The firmware admits one admin at a time; a second login is refused.
			reply(w, false, "exceeded_max_login_count")
			return
		}
		f.Logins++
		stok := fmt.Sprintf("tok%d", f.Logins)
		f.active[stok] = true
		f.MaxActive = max(f.MaxActive, len(f.active))
		http.SetCookie(w, &http.Cookie{Name: "sysauth", Value: "s" + stok, Path: "/"})
		reply(w, true, map[string]any{"stok": stok})
	default:
		http.NotFound(w, r)
	}
}

func (f *Fake) reservations(w http.ResponseWriter, op string, form url.Values) {
	switch op {
	case "load":
		if len(f.Reservations) == 0 {
			reply(w, true, map[string]any{})
			return
		}
		rows := make([]map[string]string, len(f.Reservations))
		for i, r := range f.Reservations {
			name := r.Hostname
			if n, ok := f.Names[r.MAC]; ok {
				name = n
			}
			enabled := "off"
			if r.Enabled {
				enabled = "on"
			}
			rows[i] = map[string]string{"mac": r.MAC, "ip": r.IP, "hostname": name, "enable": enabled}
		}
		reply(w, true, rows)
	case "insert":
		var row map[string]string
		if err := json.Unmarshal([]byte(form.Get("new")), &row); err != nil {
			reply(w, false, "bad_row")
			return
		}
		f.Reservations = append([]Reservation{{MAC: row["mac"], IP: row["ip"], Hostname: row["hostname"], Enabled: row["enable"] == "on"}}, f.Reservations...)
		reply(w, true, map[string]any{})
	case "remove":
		// `key` is ignored, exactly as on the real firmware: only the position counts.
		i, err := strconv.Atoi(form.Get("index"))
		if err != nil || i < 0 || i >= len(f.Reservations) {
			reply(w, false, "bad_index")
			return
		}
		f.Reservations = append(f.Reservations[:i], f.Reservations[i+1:]...)
		reply(w, true, map[string]any{})
	default:
		reply(w, false, "unsupported_operation")
	}
}

func (f *Fake) deviceName(w http.ResponseWriter, form url.Values) {
	// An empty alias answers success and changes nothing.
	if alias := form.Get("alias"); alias != "" {
		f.Names[form.Get("mac")] = alias
	}
	reply(w, true, map[string]any{})
}

func (f *Fake) generic(w http.ResponseWriter, target, op string, form url.Values) {
	if rec, ok := f.Forms[target]; ok {
		switch op {
		case "read":
			reply(w, true, rec)
		case "write":
			next := map[string]string{}
			for k := range form {
				if k != "operation" {
					next[k] = form.Get(k)
				}
			}
			f.Forms[target] = next // whole-record replace, like the firmware
			reply(w, true, map[string]any{})
		default:
			reply(w, false, "unsupported_operation")
		}
		return
	}
	if rows, ok := f.Tables[target]; ok {
		switch op {
		case "load":
			if len(rows) == 0 {
				reply(w, true, map[string]any{})
				return
			}
			reply(w, true, rows)
		case "remove":
			i, err := strconv.Atoi(form.Get("index"))
			if err != nil || i < 0 || i >= len(rows) {
				reply(w, false, "bad_index")
				return
			}
			f.Tables[target] = append(rows[:i], rows[i+1:]...)
			reply(w, true, map[string]any{})
		default:
			reply(w, false, "unsupported_operation")
		}
		return
	}
	reply(w, false, "unknown_endpoint")
}
