package router

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTablesFailClosed(t *testing.T) {
	for _, data := range []string{`null`, ``, `{"unexpected":[]}`, `"wrong"`, `[null]`, `[7]`} {
		t.Run(data, func(t *testing.T) { _, err := decodeRows(json.RawMessage(data)); require.Error(t, err) })
	}
	for _, data := range []string{`{}`, `[]`, `[{"name":"a"}]`} {
		t.Run(data, func(t *testing.T) { _, err := decodeRows(json.RawMessage(data)); require.NoError(t, err) })
	}
}

func TestFormsRejectNonRecords(t *testing.T) {
	for _, data := range []string{`null`, `[]`, `{}`, `{"enable":null}`, `{"enable":{}}`} {
		_, err := decodeForm(json.RawMessage(data))
		require.Error(t, err, data)
	}
}
