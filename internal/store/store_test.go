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

func TestQueryReadsAndTruncates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := Open(p)
	db.IngestResults(DatasetMeta{Source: "nec", PublicDataPk: "1"}, sampleResults())
	db.Close()

	ro, err := OpenReadOnly(p)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer ro.Close()

	qr, err := ro.Query("SELECT sido, votes FROM results ORDER BY id", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(qr.Columns) != 2 || qr.Columns[0] != "sido" {
		t.Errorf("columns = %v", qr.Columns)
	}
	if len(qr.Rows) != 1 || !qr.Truncated {
		t.Errorf("rows=%d truncated=%v (want 1 / true)", len(qr.Rows), qr.Truncated)
	}
}

func TestQueryRejectsWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := Open(p)
	db.Close()
	ro, _ := OpenReadOnly(p)
	defer ro.Close()
	if _, err := ro.Query("INSERT INTO datasets(source, ingested_at) VALUES('x','y')", 10); err == nil {
		t.Fatal("expected write SQL to be rejected on read-only DB")
	}
}

// v_agg_sgg 뷰와 nec.Aggregate(AggSgg) 는 같은 정의의 두 구현이다. 같은 픽스처에서
// 지표·투표율이 일치해야 정의 드리프트가 없다.
func TestViewMatchesAggregate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := Open(p)
	recs := sampleResults()
	db.IngestResults(DatasetMeta{Source: "nec", PublicDataPk: "1"}, recs)
	db.Close()

	// Go 경로: 선거구 집계 (by-votetype=false → vote_type 합쳐짐).
	aggs := nec.Aggregate(recs, nec.AggSgg, false)
	want := map[string][3]int{} // key = sido|sgg → {electorate, votes, invalid}
	for _, a := range aggs {
		want[a.Sido+"|"+a.District] = [3]int{a.Electorate, a.Votes, a.Invalid}
	}

	// SQL 경로: v_agg_sgg 는 vote_type 별이므로 sido,sgg 로 다시 합산해 비교.
	ro, _ := OpenReadOnly(p)
	defer ro.Close()
	qr, err := ro.Query(
		`SELECT sido, sgg, SUM(electorate), SUM(votes), SUM(invalid)
		 FROM v_agg_sgg GROUP BY sido, sgg ORDER BY sido, sgg`, 100)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(qr.Rows) != len(want) {
		t.Fatalf("행수 불일치: sql=%d go=%d", len(qr.Rows), len(want))
	}
	for _, row := range qr.Rows {
		key := toStr(row[0]) + "|" + toStr(row[1])
		got := [3]int{toInt(row[2]), toInt(row[3]), toInt(row[4])}
		if got != want[key] {
			t.Errorf("%s: sql=%v go=%v", key, got, want[key])
		}
	}
}

func toStr(v any) string { s, _ := v.(string); return s }
func toInt(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	}
	return 0
}
