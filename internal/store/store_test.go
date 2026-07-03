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
// 파생값(valid_votes·turnout) 까지 정확히 일치해야 정의 드리프트가 없다 — raw sum 만
// 비교하면 두 구현이 서로 다른 파생 공식을 써도 통과해버리는 구멍이 있었다(hollow test).
func TestViewMatchesAggregate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := Open(p)
	recs := sampleResults()
	// 두 번째 그룹: 완전히 별개 선거구(강남구)를 electorate=0 인 투표구로만 구성해
	// v_agg_sgg 의 "electorate=0 → turnout NULL" 가드와 nec.Aggregate 의
	// "electorate=0 → turnout 0.0" 를 같은 시나리오에서 맞대어 검증한다
	// (관외사전투표처럼 electorate 를 별도 집계하지 않는 투표구 유형을 흉내).
	recs = append(recs, nec.ResultRecord{
		Sido: "서울", District: "강남구", Town: "관외사전투표", Booth: "관외사전투표", VoteType: "관외사전",
		Electorate: 0, Votes: 15, Invalid: 2, Abstention: 0,
		Candidates: []nec.CandidateVote{{Party: "A당", Name: "김", Votes: 9}, {Party: "B당", Name: "이", Votes: 4}},
	})
	db.IngestResults(DatasetMeta{Source: "nec", PublicDataPk: "1"}, recs)
	db.Close()

	// Go 경로: 선거구 집계 (by-votetype=false → vote_type 합쳐짐). newAgg 의 그룹 정의가
	// aggregate.go 의 유일한 정의 소스이므로, 여기서 다시 계산하지 않고 그 출력을 그대로 쓴다.
	aggs := nec.Aggregate(recs, nec.AggSgg, false)
	type want struct {
		electorate, votes, invalid, validVotes int
		turnout                                float64
	}
	wantByKey := map[string]want{}
	for _, a := range aggs {
		wantByKey[a.Sido+"|"+a.District] = want{
			electorate: a.Electorate, votes: a.Votes, invalid: a.Invalid,
			validVotes: a.ValidVotes, turnout: a.Turnout,
		}
	}

	// SQL 경로: v_agg_sgg 는 vote_type 별이므로 sido,sgg 로 다시 합산해 비교.
	// turnout 은 v_agg_sgg 안에서 이미 vote_type 별로 계산돼있어 그대로 합산할 수 없으므로
	// (분모가 vote_type 마다 다름) SUM(votes)/SUM(electorate) 로 sgg 단위 재계산한다 —
	// 이것이 aggregate.go 의 Turnout = Votes/Electorate 정의와 동일한 식이다.
	ro, _ := OpenReadOnly(p)
	defer ro.Close()
	qr, err := ro.Query(
		`SELECT sido, sgg,
		        SUM(electorate) AS electorate,
		        SUM(votes) AS votes,
		        SUM(invalid) AS invalid,
		        SUM(votes) - SUM(invalid) AS valid_votes,
		        CASE WHEN SUM(electorate) > 0
		             THEN CAST(SUM(votes) AS REAL) / SUM(electorate) END AS turnout
		 FROM v_agg_sgg GROUP BY sido, sgg ORDER BY sido, sgg`, 100)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(qr.Rows) != len(wantByKey) {
		t.Fatalf("행수 불일치: sql=%d go=%d", len(qr.Rows), len(wantByKey))
	}
	const epsilon = 1e-9
	for _, row := range qr.Rows {
		key := toStr(row[0]) + "|" + toStr(row[1])
		w, ok := wantByKey[key]
		if !ok {
			t.Errorf("%s: sql 에만 존재, go 집계에 없음", key)
			continue
		}
		gotElectorate, gotVotes, gotInvalid, gotValid := toInt(row[2]), toInt(row[3]), toInt(row[4]), toInt(row[5])
		if gotElectorate != w.electorate || gotVotes != w.votes || gotInvalid != w.invalid {
			t.Errorf("%s: raw sql=(elec=%d votes=%d invalid=%d) go=(elec=%d votes=%d invalid=%d)",
				key, gotElectorate, gotVotes, gotInvalid, w.electorate, w.votes, w.invalid)
		}
		if gotValid != w.validVotes {
			t.Errorf("%s: valid_votes sql=%d go=%d", key, gotValid, w.validVotes)
		}
		// turnout: electorate=0 이면 SQL 은 NULL(→Query 가 nil 반환), Go 는 0.0 —
		// 둘 다 "계산 불가/0" 을 뜻하는 같은 의미이므로 NULL→0 로 정규화해 비교한다.
		gotTurnout := 0.0
		if row[6] != nil {
			gotTurnout = toFloat(row[6])
		}
		if diff := gotTurnout - w.turnout; diff < -epsilon || diff > epsilon {
			t.Errorf("%s: turnout sql=%v go=%v (electorate=%d)", key, gotTurnout, w.turnout, w.electorate)
		}
		if w.electorate == 0 && (gotTurnout != 0 || w.turnout != 0) {
			t.Errorf("%s: electorate=0 가드 실패 — sql=%v go=%v (둘 다 0 이어야 함)", key, gotTurnout, w.turnout)
		}
	}
}

func sampleTurnout() []nec.TurnoutAnalysisRecord {
	return []nec.TurnoutAnalysisRecord{
		{Election: "제22대 총선", Category: "표본-일반", RegionLevel: "구시군", Sido: "서울",
			Region: "전체", Gender: "합계", AgeGroup: "합계", Electorate: 710801, Voters: 493617, Rate: 69.4},
		{Election: "제22대 총선", Category: "표본-일반", RegionLevel: "구시군", Sido: "서울",
			Region: "전체", Gender: "남자", AgeGroup: "18세", Electorate: 0, Voters: 0, Rate: 0},
	}
}

func TestIngestTurnoutIdempotent(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "k.db"))
	defer db.Close()
	meta := DatasetMeta{Source: "nec", PublicDataPk: "15143936", Name: "투표율분석.zip"}
	if _, err := db.IngestTurnout(meta, sampleTurnout()); err != nil {
		t.Fatalf("ingest 1: %v", err)
	}
	if _, err := db.IngestTurnout(meta, sampleTurnout()); err != nil {
		t.Fatalf("ingest 2: %v", err)
	}
	var n int
	db.SQL().QueryRow("SELECT count(*) FROM turnout").Scan(&n)
	if n != 2 {
		t.Errorf("재적재 후 turnout=%d, want 2 (중복 없음)", n)
	}
	// v_turnout_derived: electorate>0 → rate_computed 계산, electorate=0 → NULL
	var rc *float64
	db.SQL().QueryRow(`SELECT rate_computed FROM v_turnout_derived
		WHERE region='전체' AND gender='합계' AND age_group='합계'`).Scan(&rc)
	if rc == nil {
		t.Fatal("rate_computed nil for electorate>0")
	}
	want := float64(493617) / 710801 * 100
	if *rc < want-1e-6 || *rc > want+1e-6 {
		t.Errorf("rate_computed = %v, want %v", *rc, want)
	}
	var rc0 *float64
	db.SQL().QueryRow(`SELECT rate_computed FROM v_turnout_derived WHERE age_group='18세'`).Scan(&rc0)
	if rc0 != nil {
		t.Errorf("rate_computed should be NULL for electorate=0, got %v", *rc0)
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
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	}
	return 0
}
