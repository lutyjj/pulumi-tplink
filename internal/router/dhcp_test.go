package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReservationRowsRejectMalformedFields(t *testing.T) {
	for _, field := range []string{"mac", "ip", "hostname", "enable"} {
		for _, bad := range []any{nil, 7, true, map[string]any{}} {
			t.Run(field, func(t *testing.T) {
				row := map[string]any{"mac": "aa:bb:cc:dd:ee:01", "ip": "192.0.2.11", "hostname": "", "enable": "on"}
				row[field] = bad
				_, err := decodeReservations([]map[string]any{row})
				require.Error(t, err)
			})
		}
	}
	for _, change := range []map[string]any{{"mac": ""}, {"ip": "::1"}, {"ip": "bad"}, {"enable": "yes"}} {
		row := map[string]any{"mac": "aa:bb:cc:dd:ee:01", "ip": "192.0.2.11", "hostname": "", "enable": "on"}
		for k, v := range change {
			row[k] = v
		}
		_, err := decodeReservations([]map[string]any{row})
		require.Error(t, err)
	}
	row := map[string]any{"mac": "aa:bb:cc:dd:ee:01", "ip": "192.0.2.11", "hostname": "", "enable": "off"}
	rows, err := decodeReservations([]map[string]any{row})
	require.NoError(t, err)
	assert.Equal(t, []Reservation{{MAC: "AA-BB-CC-DD-EE-01", IP: "192.0.2.11", Enabled: false}}, rows)
	_, err = decodeReservations([]map[string]any{row, row})
	require.ErrorContains(t, err, "duplicate MAC")
}
