package sqlutil

import (
	"testing"
)

func TestInt64IN(t *testing.T) {
	cond, args, next := Int64IN("id", []int64{10, 20}, 1)
	if cond != "id IN (?1,?2)" {
		t.Fatalf("cond: %q", cond)
	}
	if len(args) != 2 || args[0].(int64) != 10 || args[1].(int64) != 20 {
		t.Fatalf("args: %#v", args)
	}
	if next != 3 {
		t.Fatalf("next: %d", next)
	}
	cond, _, _ = Int64IN("x", nil, 5)
	if cond != "FALSE" {
		t.Fatalf("empty: %q", cond)
	}
}

func TestStringIN(t *testing.T) {
	cond, args, next := StringIN("ref", []string{"a", "b"}, 1)
	if cond != "ref IN (?1,?2)" {
		t.Fatalf("cond: %q", cond)
	}
	if len(args) != 2 {
		t.Fatalf("args: %#v", args)
	}
	if next != 3 {
		t.Fatalf("next: %d", next)
	}
}
