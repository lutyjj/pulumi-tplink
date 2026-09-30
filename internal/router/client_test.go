package router_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lutyjj/pulumi-tplink/internal/router"
	"github.com/lutyjj/pulumi-tplink/internal/routertest"
)

func newClient(t *testing.T, password string) (*router.Client, *routertest.Fake) {
	t.Helper()
	fake := routertest.New(t)
	return router.New(router.Config{Host: fake.Host, Password: password, Insecure: true}), fake
}

func TestNormalizeMAC(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"aa:bb:cc:dd:ee:ff", "AA-BB-CC-DD-EE-FF", "aabb.ccdd.eeff", "aabbccddeeff"} {
		got, err := router.NormalizeMAC(in)
		require.NoError(t, err, in)
		assert.Equal(t, "AA-BB-CC-DD-EE-FF", got, in)
	}
	for _, in := range []string{"", "aa:bb", "aa:bb:cc:dd:ee:ff:00"} {
		_, err := router.NormalizeMAC(in)
		assert.Error(t, err, in)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t, "wrong")
	err := c.Do(t.Context(), func(*router.Session) error { return nil })
	require.ErrorContains(t, err, "login failed")
}

func TestSessionIsReleased(t *testing.T) {
	t.Parallel()
	c, fake := newClient(t, routertest.Password)
	for range 3 {
		require.NoError(t, c.Do(t.Context(), func(*router.Session) error { return nil }))
	}
	assert.Equal(t, 3, fake.Logins, "each Do logs in afresh")
}

func TestSessionReleasedWhenCallbackFails(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t, routertest.Password)
	boom := errors.New("boom")
	require.ErrorIs(t, c.Do(t.Context(), func(*router.Session) error { return boom }), boom)
	// A leaked session would make this login fail with exceeded_max_login_count.
	require.NoError(t, c.Do(t.Context(), func(*router.Session) error { return nil }))
}

func TestConcurrentSessionsNeverOverlap(t *testing.T) {
	t.Parallel()
	c, fake := newClient(t, routertest.Password)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			assert.NoError(t, c.Do(context.Background(), func(s *router.Session) error {
				_, err := s.Reservations(context.Background())
				return err
			}))
		})
	}
	wg.Wait()
	assert.Equal(t, 1, fake.MaxActive, "the router allows one admin session")
}

func TestReservationLifecycle(t *testing.T) {
	t.Parallel()
	c, fake := newClient(t, routertest.Password)
	ctx := t.Context()
	require.NoError(t, c.Do(ctx, func(s *router.Session) error {
		for _, r := range []struct{ mac, ip string }{
			{"AA-AA-AA-AA-AA-01", "192.0.2.11"},
			{"AA-AA-AA-AA-AA-02", "192.0.2.12"},
			{"AA-AA-AA-AA-AA-03", "192.0.2.13"},
		} {
			require.NoError(t, s.InsertReservation(ctx, router.Reservation{MAC: r.mac, IP: r.ip, DeviceName: "n", Enabled: true}))
		}
		// Insert goes to the front, so the middle device sits at index 1. Deleting it
		// must not take a neighbour with it: the firmware ignores `key`.
		require.NoError(t, s.RemoveReservation(ctx, "AA-AA-AA-AA-AA-02"))
		return s.RemoveReservation(ctx, "AA-AA-AA-AA-AA-02") // already gone: no error
	}))

	var macs []string
	for _, r := range fake.Snapshot() {
		macs = append(macs, r.MAC)
	}
	assert.Equal(t, []string{"AA-AA-AA-AA-AA-03", "AA-AA-AA-AA-AA-01"}, macs)
}

func TestDeviceNameIsSeparateFromTheRow(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t, routertest.Password)
	ctx := t.Context()
	require.NoError(t, c.Do(ctx, func(s *router.Session) error {
		const mac = "AA-AA-AA-AA-AA-01"
		require.NoError(t, s.InsertReservation(ctx, router.Reservation{MAC: mac, IP: "192.0.2.11", DeviceName: "row-name", Enabled: true}))
		require.NoError(t, s.SetDeviceName(ctx, mac, "shown-name"))
		got, ok, err := s.FindReservation(ctx, mac)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "shown-name", got.DeviceName, "the device table outranks the row")
		return nil
	}))
}

func TestAssertFormPreservesOtherFields(t *testing.T) {
	t.Parallel()
	c, fake := newClient(t, routertest.Password)
	ctx := t.Context()
	require.NoError(t, c.Do(ctx, func(s *router.Session) error {
		return s.SetToggle(ctx, router.DMZ, true)
	}))
	fake.Lock()
	defer fake.Unlock()
	assert.Equal(t, map[string]string{"enable": "on", "ipaddr": "0.0.0.0"}, fake.Forms["admin/nat?form=dmz"])
}

func TestClearIngressRulesWalksBackwards(t *testing.T) {
	t.Parallel()
	c, fake := newClient(t, routertest.Password)
	fake.Lock()
	fake.Tables["admin/nat?form=vs"] = []map[string]string{
		{"name": "a", "port": "1"}, {"name": "b", "port": "2"}, {"name": "c", "port": "3"},
	}
	fake.Unlock()
	ctx := t.Context()
	var removed []string
	require.NoError(t, c.Do(ctx, func(s *router.Session) (err error) {
		removed, err = s.ClearIngressRules(ctx, router.PortForwarding)
		return err
	}))
	assert.Equal(t, []string{"name=a port=1", "name=b port=2", "name=c port=3"}, removed)
	fake.Lock()
	defer fake.Unlock()
	assert.Empty(t, fake.Tables["admin/nat?form=vs"])
}

func TestTransportErrorRedactsTokenAndPreservesCause(t *testing.T) {
	t.Parallel()
	c, _ := newClient(t, routertest.Password)
	err := c.Do(t.Context(), func(s *router.Session) error {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := s.Reservations(ctx)
		return err
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), "stok=")
	assert.NotContains(t, err.Error(), "tok1")
	require.NoError(t, c.Do(t.Context(), func(*router.Session) error { return nil }), "cancellation still releases the admin session")
}
