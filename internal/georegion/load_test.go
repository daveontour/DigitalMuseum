package georegion

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	path := filepath.Join("testdata", "regions_test.json")
	if err := Load(path); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestLoadValidation(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("duplicate code", func(t *testing.T) {
		p := write("dup.json", `{"default_region":"oth","default_label":"Other","regions":[
			{"code":"aus","label":"A","bbox":[1,2,3,4]},
			{"code":"aus","label":"B","bbox":[5,6,7,8]}
		]}`)
		if _, err := loadRegistry(p); err == nil {
			t.Fatal("expected duplicate code error")
		}
	})

	t.Run("bad bbox length", func(t *testing.T) {
		p := write("badbbox.json", `{"default_region":"oth","default_label":"Other","regions":[
			{"code":"aus","label":"A","bbox":[1,2,3]}
		]}`)
		if _, err := loadRegistry(p); err == nil {
			t.Fatal("expected bbox length error")
		}
	})

	t.Run("min greater than max", func(t *testing.T) {
		p := write("minmax.json", `{"default_region":"oth","default_label":"Other","regions":[
			{"code":"aus","label":"A","bbox":[10,20,5,25]}
		]}`)
		if _, err := loadRegistry(p); err == nil {
			t.Fatal("expected min_lon > max_lon error")
		}
	})
}

func TestLabel(t *testing.T) {
	reg := Default()
	if got := reg.Label("aus"); got != "Australia" {
		t.Fatalf("Label(aus) = %q, want Australia", got)
	}
	if got := reg.Label("oth"); got != "Other" {
		t.Fatalf("Label(oth) = %q, want Other", got)
	}
	if got := reg.Label(""); got != "Unknown" {
		t.Fatalf("Label(empty) = %q, want Unknown", got)
	}
	if got := reg.Label("stale_code"); got != "stale_code" {
		t.Fatalf("Label(unknown) = %q, want stale_code", got)
	}
}
