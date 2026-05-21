package sqlutil

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DBTime wraps time.Time so database/sql can scan SQLite TEXT (and other driver
// string forms) into Go time values. go-sqlite3 typically returns string for TEXT.
type DBTime struct {
	time.Time
}

// Scan implements sql.Scanner.
func (t *DBTime) Scan(src interface{}) error {
	if t == nil {
		return fmt.Errorf("sqlutil.DBTime: Scan on nil pointer")
	}
	if src == nil {
		t.Time = time.Time{}
		return nil
	}
	switch v := src.(type) {
	case time.Time:
		t.Time = v
		return nil
	case []byte:
		return t.Scan(string(v))
	case string:
		if v == "" {
			t.Time = time.Time{}
			return nil
		}
		parsed, err := ParseSQLiteDatetime(v)
		if err != nil {
			return err
		}
		t.Time = parsed
		return nil
	case int64:
		t.Time = time.Unix(v, 0).UTC()
		return nil
	case float64:
		t.Time = time.Unix(int64(v), 0).UTC()
		return nil
	default:
		return fmt.Errorf("sqlutil.DBTime: cannot scan %T", src)
	}
}

// Value implements driver.Valuer.
func (t DBTime) Value() (driver.Value, error) {
	if t.Time.IsZero() {
		return nil, nil
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

// MarshalJSON encodes like time.Time (including zero as RFC3339 zero instant).
func (t DBTime) MarshalJSON() ([]byte, error) {
	return t.Time.MarshalJSON()
}

// UnmarshalJSON decodes RFC3339 JSON into DBTime.
func (t *DBTime) UnmarshalJSON(data []byte) error {
	return (*time.Time)(&t.Time).UnmarshalJSON(data)
}

// NullDBTime is a nullable instant for SQL columns and JSON (like sql.NullTime,
// but scans driver string values from SQLite).
type NullDBTime struct {
	Time  time.Time
	Valid bool
}

// Scan implements sql.Scanner.
func (n *NullDBTime) Scan(src interface{}) error {
	if src == nil {
		n.Time, n.Valid = time.Time{}, false
		return nil
	}
	var dt DBTime
	if err := dt.Scan(src); err != nil {
		n.Time, n.Valid = time.Time{}, false
		return err
	}
	n.Time = dt.Time
	n.Valid = true
	return nil
}

// Value implements driver.Valuer.
func (n NullDBTime) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return DBTime{Time: n.Time}.Value()
}

// MarshalJSON emits JSON null when invalid.
func (n NullDBTime) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return n.Time.MarshalJSON()
}

// UnmarshalJSON loads from JSON null or an RFC3339 string.
func (n *NullDBTime) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		n.Valid = false
		n.Time = time.Time{}
		return nil
	}
	if err := json.Unmarshal(data, &n.Time); err != nil {
		return err
	}
	n.Valid = true
	return nil
}

// Ptr returns a heap-allocated time pointer, or nil when invalid (for call sites
// that still expect *time.Time).
func (n NullDBTime) Ptr() *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
}

func trimGoMonotonicSuffix(s string) string {
	// SQLite TEXT may store time.Time.String(), including a monotonic clock suffix
	// like " ... m=+12.345678901".
	if i := strings.Index(s, " m=+"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// ParseSQLiteDatetime parses timestamps stored as TEXT (or similar) in SQLite.
func ParseSQLiteDatetime(s string) (time.Time, error) {
	s = trimGoMonotonicSuffix(s)
	layouts := []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",   // Go time.String (wall clock; no monotonic)
		"2006-01-02 15:04:05.999999999 -0700 -0700", // Go time.String when tz has no alphabetic name
		"2006-01-02 15:04:05 -0700 -0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05.999999999+00:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999",
		"2006-01-02T15:04:05",
	}
	var lastErr error
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, fmt.Errorf("parse sqlite datetime %q: %w", s, lastErr)
}
