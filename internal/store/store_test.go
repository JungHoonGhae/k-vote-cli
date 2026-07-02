package store

import (
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
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

func sampleResults() []nec.ResultRecord {
	return []nec.ResultRecord{
		{Sido: "서울", District: "종로구", Town: "청운효자동", Booth: "제1투", VoteType: "본투표",
			Electorate: 100, Votes: 80, Invalid: 5, Abstention: 20,
			Candidates: []nec.CandidateVote{{Party: "A당", Name: "김", Votes: 40}, {Party: "B당", Name: "이", Votes: 35}}},
		{Sido: "서울", District: "종로구", Town: "관내사전투표", Booth: "관내사전투표", VoteType: "관내사전",
			Electorate: 0, Votes: 20, Invalid: 1, Abstention: 0,
			Candidates: []nec.CandidateVote{{Party: "A당", Name: "김", Votes: 12}, {Party: "B당", Name: "이", Votes: 7}}},
	}
}

func TestIngestResultsIdempotent(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "k.db"))
	defer db.Close()
	meta := DatasetMeta{Source: "nec", PublicDataPk: "123", Name: "총선개표.csv"}

	if _, err := db.IngestResults(meta, sampleResults()); err != nil {
		t.Fatalf("ingest 1: %v", err)
	}
	if _, err := db.IngestResults(meta, sampleResults()); err != nil {
		t.Fatalf("ingest 2: %v", err)
	}
	var ds, rs, cs int
	db.SQL().QueryRow("SELECT count(*) FROM datasets").Scan(&ds)
	db.SQL().QueryRow("SELECT count(*) FROM results").Scan(&rs)
	db.SQL().QueryRow("SELECT count(*) FROM candidates").Scan(&cs)
	if ds != 1 || rs != 2 || cs != 4 {
		t.Errorf("재적재 후 중복: datasets=%d results=%d candidates=%d (want 1/2/4)", ds, rs, cs)
	}
}

func TestIngestPollsIdempotent(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "k.db"))
	defer db.Close()
	meta := DatasetMeta{Source: "nesdc", PublicDataPk: "bulk", Name: "누적.xlsx"}
	recs := []nesdc.PollRecord{
		{Period: "2024", Agency: "갤럽", SampleSize: "1000",
			PartySupport: map[string]string{"A당": "40", "B당": "35"}},
	}
	if _, err := db.IngestPolls(meta, recs); err != nil {
		t.Fatalf("ingest 1: %v", err)
	}
	if _, err := db.IngestPolls(meta, recs); err != nil {
		t.Fatalf("ingest 2: %v", err)
	}
	var ps, pp int
	db.SQL().QueryRow("SELECT count(*) FROM polls").Scan(&ps)
	db.SQL().QueryRow("SELECT count(*) FROM party_support").Scan(&pp)
	if ps != 1 || pp != 2 {
		t.Errorf("재적재 후 중복: polls=%d party_support=%d (want 1/2)", ps, pp)
	}
}
