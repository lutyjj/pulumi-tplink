// Package router speaks the undocumented web API of TP-Link Archer routers.
//
// The protocol is verified on an Archer AXE75, firmware 1.10.5.
// docs/protocol.md explains the login
// handshake and the firmware quirks this package works around.
package router

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// requestTimeout bounds one HTTP exchange with the router.
const requestTimeout = 15 * time.Second

// luci is the prefix every endpoint hangs off; the session token follows it.
const luci = "/cgi-bin/luci/;stok="

// Config identifies one router and how to authenticate to it.
type Config struct {
	// Host is the router's LAN address, with an optional port.
	Host string
	// Password is the local admin password, as typed into the web UI.
	Password string
	// Insecure skips TLS verification. The router serves a self-signed certificate.
	Insecure bool
}

// Client talks to one router. It is safe for concurrent use: sessions to the same
// router are serialised, because the firmware allows a single admin login.
type Client struct {
	cfg  Config
	http *http.Client
}

// New returns a client for the router described by cfg.
func New(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout:       requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("router redirects are not allowed") },
			Transport: &http.Transport{
				// The bypass is scoped to this client, never to the process.
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.Insecure}, //nolint:gosec // opt-in, see Config.Insecure
			},
		},
	}
}

// sessionLocks holds one cancellable slot per router host, shared by every Client so that two
// resources racing for the admin slot queue up instead of colliding.
var sessionLocks sync.Map

func lockFor(host string) chan struct{} {
	slot, _ := sessionLocks.LoadOrStore(strings.ToLower(host), make(chan struct{}, 1))
	return slot.(chan struct{})
}

// Target addresses one endpoint: `<Path>?form=<Form>`.
type Target struct {
	Path string
	Form string
}

// Envelope is the uniform response wrapper of every endpoint.
type Envelope struct {
	Success   bool            `json:"success"`
	ErrorCode any             `json:"errorcode"`
	Data      json.RawMessage `json:"data"`
}

// Session is an authenticated admin session. It is only valid inside the callback
// passed to [Client.Do].
type Session struct {
	c      *Client
	stok   string
	cookie string
}

// Do logs in, runs fn, and logs out again. Sessions to one router never overlap.
func (c *Client) Do(ctx context.Context, fn func(*Session) error) error {
	slot := lockFor(c.cfg.Host)
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-slot }()

	s, err := c.login(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// Free the router's single admin slot even when ctx is already cancelled.
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
		defer cancel()
		// Best effort: an unreleased session times out on its own.
		_, _ = s.post(lctx, Target{"admin/system", "logout"}, url.Values{"operation": {"write"}})
	}()
	return fn(s)
}

func (c *Client) base() string { return "https://" + c.cfg.Host }

func (c *Client) rawPost(ctx context.Context, path, cookie string, body url.Values) (*Envelope, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+path, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, "", errors.New("invalid router request URL")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", c.base()+"/webpages/index.html")
	req.Header.Set("Origin", c.base())
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// net/http wraps failures in url.Error, whose message includes the secret
		// session token in the URL. Keep the cause for errors.Is, not the URL.
		var requestErr *url.Error
		if errors.As(err, &requestErr) {
			err = requestErr.Err
		}
		return nil, "", fmt.Errorf("router request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("router returned HTTP %d", resp.StatusCode)
	}
	const maxBody = 8 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > maxBody {
		return nil, "", errors.New("router response exceeded 8 MiB")
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, "", fmt.Errorf("non-JSON response from router")
	}
	var setCookie string
	if v := resp.Header.Values("Set-Cookie"); len(v) > 0 {
		setCookie, _, _ = strings.Cut(v[0], ";")
	}
	return &env, setCookie, nil
}

// login performs the RSA-only handshake described in docs/protocol.md.
func (c *Client) login(ctx context.Context) (*Session, error) {
	keys, _, err := c.rawPost(ctx, luci+"/login?form=keys", "", url.Values{"operation": {"read"}})
	if err != nil {
		return nil, err
	}
	var pub struct {
		Password []string `json:"password"`
	}
	if err := json.Unmarshal(keys.Data, &pub); err != nil || !keys.Success || len(pub.Password) != 2 {
		return nil, errors.New("router did not return a login key")
	}
	encrypted, err := encryptPassword(pub.Password[0], pub.Password[1], c.cfg.Password)
	if err != nil {
		return nil, err
	}

	// The hex ciphertext needs no escaping, so it is sent verbatim like the web UI does.
	env, cookie, err := c.rawPost(ctx, luci+"/login?form=login", "",
		url.Values{"operation": {"login"}, "password": {encrypted}})
	if err != nil {
		return nil, err
	}
	if !env.Success {
		return nil, fmt.Errorf("router login failed: %v", env.ErrorCode)
	}
	var data struct {
		Stok string `json:"stok"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil || data.Stok == "" {
		return nil, errors.New("router login returned no session token")
	}
	return &Session{c: c, stok: data.Stok, cookie: cookie}, nil
}

// encryptPassword encrypts the admin password with the router's RSA public key
// (PKCS#1 v1.5) and returns it hex-encoded, as the web UI does.
func encryptPassword(nHex, eHex, password string) (string, error) {
	if len(eHex)%2 == 1 {
		eHex = "0" + eHex
	}
	n, ok := new(big.Int).SetString(nHex, 16)
	if !ok || n.Sign() <= 0 || n.BitLen() < 1024 || n.BitLen() > 8192 {
		return "", errors.New("router login key: invalid modulus")
	}
	e, ok := new(big.Int).SetString(eHex, 16)
	if !ok || !e.IsInt64() || e.Int64() < 3 || e.Int64() > 2147483647 || e.Bit(0) == 0 {
		return "", errors.New("router login key: exponent is not hex")
	}
	ct, err := rsa.EncryptPKCS1v15(rand.Reader, &rsa.PublicKey{N: n, E: int(e.Int64())}, []byte(password)) //nolint:staticcheck // Firmware requires PKCS#1 v1.5; OAEP is not wire-compatible.
	if err != nil {
		return "", fmt.Errorf("encrypt router password: %w", err)
	}
	return hex.EncodeToString(ct), nil
}

// post sends one form-encoded request and returns the envelope without judging it.
func (s *Session) post(ctx context.Context, t Target, body url.Values) (*Envelope, error) {
	path := luci + s.stok + "/" + t.Path + "?form=" + t.Form
	env, _, err := s.c.rawPost(ctx, path, s.cookie, body)
	return env, err
}

// call sends one request and fails unless the router reports success.
func (s *Session) call(ctx context.Context, what string, t Target, body url.Values) (*Envelope, error) {
	env, err := s.post(ctx, t, body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if !env.Success {
		return nil, fmt.Errorf("%s failed: %v", what, env.ErrorCode)
	}
	return env, nil
}
