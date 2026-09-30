package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Form is one settings record as the router holds it: every field a string.
type Form map[string]string

// ReadForm fetches a record with the `read` operation.
func (s *Session) ReadForm(ctx context.Context, t Target) (Form, error) {
	env, err := s.call(ctx, fmt.Sprintf("read %s?form=%s", t.Path, t.Form), t, url.Values{"operation": {"read"}})
	if err != nil {
		return nil, err
	}
	return decodeForm(env.Data)
}

// WriteForm replaces a record with the `write` operation. The firmware replaces the
// whole record, so callers must pass every field back: read first and merge.
func (s *Session) WriteForm(ctx context.Context, t Target, values Form) error {
	body := url.Values{"operation": {"write"}}
	for k, v := range values {
		body.Set(k, v)
	}
	_, err := s.call(ctx, fmt.Sprintf("write %s?form=%s", t.Path, t.Form), t, body)
	return err
}

// AssertForm sets the given fields on a record and leaves every other field as the
// router holds it.
func (s *Session) AssertForm(ctx context.Context, t Target, values Form) error {
	current, err := s.ReadForm(ctx, t)
	if err != nil {
		return err
	}
	for k, v := range values {
		current[k] = v
	}
	return s.WriteForm(ctx, t, current)
}

// decodeForm flattens a JSON object into string fields. The firmware sends strings,
// but a number or bool must not make a read fail.
func decodeForm(data json.RawMessage) (Form, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) == 0 {
		return nil, fmt.Errorf("expected a nonempty settings record")
	}
	form := make(Form, len(raw))
	for k, v := range raw {
		if str, ok := v.(string); ok {
			form[k] = str
		} else {
			switch v.(type) {
			case bool, float64:
				form[k] = fmt.Sprint(v)
			default:
				return nil, fmt.Errorf("settings field %q is not scalar", k)
			}
		}
	}
	return form, nil
}

// decodeRows reads a table. The router answers `{}` for an empty table and a JSON
// array once it holds rows; anything else is refused rather than read as empty,
// because a caller asserting "no rows" must not be told a table is clear merely
// because its shape was unrecognised.
func decodeRows(data json.RawMessage) ([]map[string]any, error) {
	trimmed := strings.TrimSpace(string(data))
	switch {
	case strings.HasPrefix(trimmed, "["):
		var rows []map[string]any
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("unreadable table rows: %w", err)
		}
		for _, row := range rows {
			if row == nil {
				return nil, fmt.Errorf("table contained a null row")
			}
		}
		return rows, nil
	case trimmed == "{}":
		return nil, nil
	default:
		return nil, fmt.Errorf("expected a list of rows or an empty object")
	}
}

// loadRows fetches a table with the `load` operation.
func (s *Session) loadRows(ctx context.Context, what string, t Target) ([]map[string]any, error) {
	env, err := s.call(ctx, what, t, url.Values{"operation": {"load"}})
	if err != nil {
		return nil, err
	}
	rows, err := decodeRows(env.Data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return rows, nil
}

// str reads a row field as a string, treating absence as empty.
func str(row map[string]any, key string) string {
	if v, ok := row[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprint(v)
	}
	return ""
}

// summarise describes a row without claiming to know its schema, with fields in a
// stable order so the same row always reads the same in a log line or a diff.
func summarise(row map[string]any) string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		if v := str(row, k); v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	if len(parts) == 0 {
		return "(empty row)"
	}
	return strings.Join(parts, " ")
}
