package provider_test

import (
	"github.com/blang/semver"
	"github.com/lutyjj/pulumi-tplink/internal/routertest"
	"github.com/lutyjj/pulumi-tplink/provider"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/integration"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func props(values map[string]any) property.Map {
	return resource.FromResourcePropertyMap(resource.NewPropertyMapFromMap(values))
}

func urn(kind string) resource.URN {
	return resource.NewURN("test", "tplink", "", "tplink:index:"+tokens.Type(kind), "test")
}

func server(t *testing.T) (integration.Server, *routertest.Fake) {
	t.Helper()
	f := routertest.New(t)
	s, err := integration.NewServer(t.Context(), "tplink", semver.Version{Minor: 1}, integration.WithProvider(provider.Provider()))
	require.NoError(t, err)
	c, err := s.CheckConfig(p.CheckRequest{Inputs: props(map[string]any{"host": f.Host, "password": routertest.Password, "insecure": true})})
	require.NoError(t, err)
	require.Empty(t, c.Failures)
	require.True(t, c.Inputs.Get("password").Secret(), "password must always be secret")
	require.NoError(t, s.Configure(p.ConfigureRequest{Args: c.Inputs}))
	return s, f
}

func check(t *testing.T, s integration.Server, kind string, values map[string]any) property.Map {
	t.Helper()
	res, err := s.Check(p.CheckRequest{Urn: urn(kind), Inputs: props(values)})
	require.NoError(t, err)
	require.Empty(t, res.Failures)
	return res.Inputs
}

func TestReservationLifecycle(t *testing.T) {
	t.Parallel()
	s, f := server(t)
	integration.LifeCycleTest{
		Resource: "tplink:index:DhcpReservation",
		Create: integration.Operation{Inputs: props(map[string]any{"mac": "aa:bb:cc:dd:ee:01", "ip": "192.0.2.11", "deviceName": "nas"}), Hook: func(_, output property.Map) {
			assert.Equal(t, "AA-BB-CC-DD-EE-01", output.Get("mac").AsString())
			require.Len(t, f.Snapshot(), 1)
		}},
		Updates: []integration.Operation{
			{Inputs: props(map[string]any{"mac": "AA-BB-CC-DD-EE-01", "ip": "192.0.2.12", "deviceName": "new-name"}), Hook: func(_, output property.Map) {
				require.Len(t, f.Snapshot(), 1)
				assert.Equal(t, "192.0.2.12", f.Snapshot()[0].IP)
				assert.Equal(t, "new-name", output.Get("deviceName").AsString())
			}},
		},
	}.Run(t, s)
	assert.Empty(t, f.Snapshot())
	assert.Equal(t, "new-name", f.Names["AA-BB-CC-DD-EE-01"], "deletion leaves the device name")
}

func TestPreviewNeverWritesOrLogsIn(t *testing.T) {
	t.Parallel()
	s, f := server(t)
	cases := map[string]map[string]any{
		"DhcpReservation": {"mac": "AA-BB-CC-DD-EE-01", "ip": "192.0.2.11"},
		"DhcpServer":      {"primary": "192.0.2.2"},
		"Upnp":            {"enabled": false}, "Dmz": {"enabled": false}, "RemoteAdmin": {"enabled": false}, "InboundRules": {},
	}
	for kind, values := range cases {
		inputs := check(t, s, kind, values)
		out, err := s.Create(p.CreateRequest{Urn: urn(kind), Properties: inputs, DryRun: true})
		require.NoError(t, err)
		_, err = s.Update(p.UpdateRequest{Urn: urn(kind), ID: out.ID, State: out.Properties, Inputs: inputs, DryRun: true})
		require.NoError(t, err)
	}
	assert.Zero(t, f.Logins)
}

func TestValidationAndUnknownInputs(t *testing.T) {
	t.Parallel()
	s, _ := server(t)
	for _, values := range []map[string]any{
		{"mac": "garbage-aa-bb-cc-dd-ee-01", "ip": "192.0.2.11"},
		{"mac": "AA-BB-CC-DD-EE-01", "ip": "999.0.2.11"},
		{"mac": "AA-BB-CC-DD-EE-01", "ip": "192.0.2.11", "deviceName": "   "},
	} {
		out, err := s.Check(p.CheckRequest{Urn: urn("DhcpReservation"), Inputs: props(values)})
		require.NoError(t, err)
		require.NotEmpty(t, out.Failures)
	}
	in := property.NewMap(map[string]property.Value{"mac": property.New(property.Computed), "ip": property.New(property.Computed), "deviceName": property.New(property.Computed)})
	out, err := s.Check(p.CheckRequest{Urn: urn("DhcpReservation"), Inputs: in})
	require.NoError(t, err)
	require.Empty(t, out.Failures)
	assert.True(t, out.Inputs.Get("mac").IsComputed())
	assert.True(t, out.Inputs.Get("ip").IsComputed())
}

func TestReservationAdoptionReadAndReplacement(t *testing.T) {
	t.Parallel()
	s, f := server(t)
	const mac = "AA-BB-CC-DD-EE-01"
	f.Reservations = []routertest.Reservation{{MAC: mac, IP: "192.0.2.11", Hostname: "nas"}}
	in := check(t, s, "DhcpReservation", map[string]any{"mac": mac, "ip": "192.0.2.11", "deviceName": "nas"})
	out, err := s.Create(p.CreateRequest{Urn: urn("DhcpReservation"), Properties: in})
	require.NoError(t, err)
	require.Len(t, f.Snapshot(), 1, "adopt, do not insert a duplicate")
	imported, err := s.Read(p.ReadRequest{Urn: urn("DhcpReservation"), ID: out.ID})
	require.NoError(t, err)
	assert.Equal(t, mac, imported.Inputs.Get("mac").AsString())
	// Refresh reports actual state, and a missing row signals deletion.
	f.Reservations[0].IP = "192.0.2.50"
	read, err := s.Read(p.ReadRequest{Urn: urn("DhcpReservation"), ID: out.ID, Inputs: in, Properties: out.Properties})
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.50", read.Properties.Get("ip").AsString())
	different := check(t, s, "DhcpReservation", map[string]any{"mac": "AA-BB-CC-DD-EE-02", "ip": "192.0.2.11"})
	diff, err := s.Diff(p.DiffRequest{Urn: urn("DhcpReservation"), ID: out.ID, State: out.Properties, Inputs: different})
	require.NoError(t, err)
	assert.Equal(t, p.UpdateReplace, diff.DetailedDiff["mac"].Kind)
	assert.True(t, diff.DeleteBeforeReplace)
	f.Reservations = nil
	read, err = s.Read(p.ReadRequest{Urn: urn("DhcpReservation"), ID: out.ID, Inputs: in, Properties: out.Properties})
	require.NoError(t, err)
	assert.Empty(t, read.ID)
}

func TestFailedReaddressRestoresOldReservation(t *testing.T) {
	t.Parallel()
	s, f := server(t)
	in := check(t, s, "DhcpReservation", map[string]any{"mac": "AA-BB-CC-DD-EE-01", "ip": "192.0.2.11", "deviceName": "nas"})
	out, err := s.Create(p.CreateRequest{Urn: urn("DhcpReservation"), Properties: in})
	require.NoError(t, err)
	f.FailNext["admin/dhcps?form=reservation/insert"] = 1
	next := check(t, s, "DhcpReservation", map[string]any{"mac": "AA-BB-CC-DD-EE-01", "ip": "192.0.2.12", "deviceName": "nas"})
	_, err = s.Update(p.UpdateRequest{Urn: urn("DhcpReservation"), ID: out.ID, State: out.Properties, Inputs: next})
	require.Error(t, err)
	rows := f.Snapshot()
	require.Len(t, rows, 1)
	assert.Equal(t, "192.0.2.11", rows[0].IP)
	_, err = s.Update(p.UpdateRequest{Urn: urn("DhcpReservation"), ID: out.ID, State: out.Properties, Inputs: next})
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.12", f.Snapshot()[0].IP)
}

func TestDhcpServerPreservesUnmanagedFields(t *testing.T) {
	t.Parallel()
	s, f := server(t)
	integration.LifeCycleTest{Resource: "tplink:index:DhcpServer", Create: integration.Operation{
		Inputs: props(map[string]any{"primary": "192.0.2.2", "poolStart": "192.0.2.20", "poolEnd": "192.0.2.99"}),
		Hook: func(_, output property.Map) {
			form := f.Forms["admin/dhcps?form=setting"]
			assert.Equal(t, "120", form["leasetime"])
			assert.Equal(t, "lan", form["domain"])
			assert.Equal(t, "192.0.2.1", form["gateway"])
			assert.Equal(t, "192.0.2.20", output.Get("poolStart").AsString())
		}}, Updates: []integration.Operation{{Inputs: props(map[string]any{"primary": "192.0.2.3", "secondary": "192.0.2.4"})}}}.Run(t, s)
	assert.Equal(t, "192.0.2.3", f.Forms["admin/dhcps?form=setting"]["pri_dns"], "deletion does not blank DNS")
	assert.Equal(t, "192.0.2.20", f.Forms["admin/dhcps?form=setting"]["ipaddr_start"], "omitted pool bounds survive")
}

func TestToggleLifecycles(t *testing.T) {
	for kind, target := range map[string]string{"Upnp": "admin/upnp?form=enable", "Dmz": "admin/nat?form=dmz", "RemoteAdmin": "admin/administration?form=remote"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			s, f := server(t)
			integration.LifeCycleTest{Resource: "tplink:index:" + tokens.Type(kind), Create: integration.Operation{Inputs: props(map[string]any{"enabled": false})}, Updates: []integration.Operation{{Inputs: props(map[string]any{"enabled": true})}}}.Run(t, s)
			assert.Equal(t, "on", f.Forms[target]["enable"], "delete leaves the setting")
		})
	}
}

func TestInboundRulesDetectsLiveDriftWithoutRefresh(t *testing.T) {
	t.Parallel()
	s, f := server(t)
	out, err := s.Create(p.CreateRequest{Urn: urn("InboundRules"), Properties: property.Map{}})
	require.NoError(t, err)
	f.Tables["admin/nat?form=vs"] = []map[string]string{{"name": "a"}, {"name": "b"}, {"name": "c"}}
	f.Tables["admin/nat?form=pt"] = []map[string]string{{"name": "trigger"}}
	diff, err := s.Diff(p.DiffRequest{Urn: urn("InboundRules"), ID: out.ID, State: out.Properties, Inputs: property.Map{}})
	require.NoError(t, err)
	assert.True(t, diff.HasChanges)
	require.Len(t, f.Tables["admin/nat?form=vs"], 3, "diff only reads")
	next, err := s.Update(p.UpdateRequest{Urn: urn("InboundRules"), ID: out.ID, State: out.Properties, Inputs: property.Map{}})
	require.NoError(t, err)
	assert.Empty(t, f.Tables["admin/nat?form=vs"])
	assert.Empty(t, f.Tables["admin/nat?form=pt"])
	assert.Equal(t, 4, next.Properties.Get("removed").AsArray().Len())
	diff, err = s.Diff(p.DiffRequest{Urn: urn("InboundRules"), ID: out.ID, State: next.Properties, Inputs: property.Map{}})
	require.NoError(t, err)
	assert.False(t, diff.HasChanges)
}

func TestCredentialRotationDoesNotReplaceResources(t *testing.T) {
	t.Parallel()
	s, f := server(t)
	old := props(map[string]any{"host": f.Host, "password": "old", "insecure": true})
	next := props(map[string]any{"host": f.Host, "password": "new", "insecure": true})
	diff, err := s.DiffConfig(p.DiffRequest{State: old, Inputs: next})
	require.NoError(t, err)
	assert.Equal(t, p.Update, diff.DetailedDiff["password"].Kind)
}
