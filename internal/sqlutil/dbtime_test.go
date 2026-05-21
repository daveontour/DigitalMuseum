package sqlutil

import "testing"

func TestParseSQLiteDatetime_GoStringWithMonotonic(t *testing.T) {
	s := "2026-04-19 23:40:53.1956232 +1000 AEST m=+86440.597567801"
	if _, err := ParseSQLiteDatetime(s); err != nil {
		t.Fatal(err)
	}
}
