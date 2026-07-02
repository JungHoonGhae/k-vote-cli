package store

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesFileAndCloses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if db.SQL() == nil {
		t.Fatal("SQL() nil")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestOpenReadOnlyRejectsWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.Close()

	ro, err := OpenReadOnly(p)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer ro.Close()
	if _, err := ro.SQL().Exec("CREATE TABLE x(a)"); err == nil {
		t.Fatal("expected write to be rejected on read-only DB")
	}
}

func TestMigrateCreatesTablesAndViews(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	want := []string{"datasets", "results", "candidates", "polls", "party_support",
		"v_results_derived", "v_agg_sgg", "v_agg_sido"}
	for _, name := range want {
		var n int
		err := db.SQL().QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE name = ?`, name).Scan(&n)
		if err != nil || n != 1 {
			t.Errorf("object %q: count=%d err=%v", name, n, err)
		}
	}
}
