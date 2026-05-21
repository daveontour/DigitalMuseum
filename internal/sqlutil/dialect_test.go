package sqlutil

import (
	"context"
	"testing"
)

func TestIsSQLiteNil(t *testing.T) {
	if IsSQLite(context.Background(), nil) {
		t.Fatal("expected false for nil db")
	}
}
