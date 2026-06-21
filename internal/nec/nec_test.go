package nec

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

// listHTML mirrors the data.go.kr file-dataset listing markup: a <dl> per
// dataset whose <dt> leads with format labels and a screen-reader title, with
// the detail link carrying the publicDataPk.
const listHTML = `<html><body>
<dl>
  <dt>
    <a href="/data/15025527/fileData.do"></a>
    <span class="format">CSV</span>
    <span class="format">JSON</span> + <span class="format">XML</span>
    <span class="sr-only">중앙선거관리위원회_국회의원선거 개표결과</span> 미리보기
  </dt>
  <dd>[제22대 국회의원선거 개표결과] 2024년 4월 10일에 실시한 선거 결과</dd>
</dl>
<dl>
  <dt>
    <a href="/data/15101509/fileData.do"></a>
    <span class="format">XLSX</span>
    <span class="sr-only">중앙선거관리위원회_제8회 전국동시지방선거 개표결과</span> 미리보기
  </dt>
  <dd>[제8회 전국동시지방선거 개표결과] 2022년 6월 1일 결과</dd>
</dl>
</body></html>`

const detailHTML = `<html><body><script>
function init(){ fn_fileDataDown('15025527', 'uddi:abcd-1234-ef', '','1', '3'); }
</script></body></html>`

const resolveJSON = `{"status":true,"atchFileId":"FILE_000000003172714","fileDetailSn":1,
"dataSetFileDetailInfo":{"dataNm":"중앙선거관리위원회_국회의원선거 개표결과_20240410"}}`

func necServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tcs/dss/selectDataSetList.do":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.Write([]byte(listHTML))
		case "/data/15025527/fileData.do":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.Write([]byte(detailHTML))
		case "/tcs/dss/selectFileDataDownload.do":
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.Write([]byte(resolveJSON))
		case "/cmm/cmm/fileDownload.do":
			w.Header().Set("Content-Disposition", `attachment; filename="중앙선거관리위원회_국회의원선거 개표결과_20240410.csv"`)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("시도명,선거구명,후보자,득표수\n서울,종로구,곽상언,56\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testClient(t *testing.T, base string) *Client {
	return New(WithBaseURL(base), WithDelay(0), WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))
}

func TestDatasets(t *testing.T) {
	c := testClient(t, necServer(t).URL)
	ds, err := c.Datasets(context.Background(), SearchOptions{Keyword: "개표결과"})
	if err != nil {
		t.Fatalf("Datasets: %v", err)
	}
	if len(ds) != 2 {
		t.Fatalf("got %d datasets, want 2: %+v", len(ds), ds)
	}
	d := ds[0]
	if d.PublicDataPk != "15025527" {
		t.Errorf("pk = %q", d.PublicDataPk)
	}
	if d.Title != "중앙선거관리위원회_국회의원선거 개표결과" {
		t.Errorf("title = %q", d.Title)
	}
	if len(d.Formats) != 3 || d.Formats[0] != "CSV" {
		t.Errorf("formats = %v, want [CSV JSON XML]", d.Formats)
	}
	if ds[1].Formats[0] != "XLSX" {
		t.Errorf("second dataset formats = %v", ds[1].Formats)
	}
}

func TestResolve(t *testing.T) {
	c := testClient(t, necServer(t).URL)
	fi, err := c.Resolve(context.Background(), "15025527")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if fi.AtchFileID != "FILE_000000003172714" {
		t.Errorf("atchFileId = %q", fi.AtchFileID)
	}
	if fi.FileDetailSn != "1" {
		t.Errorf("fileDetailSn = %q, want 1", fi.FileDetailSn)
	}
	if fi.Name == "" {
		t.Error("missing name")
	}
}

func TestParseResults(t *testing.T) {
	// Two polling units that share the same (시도,선거구,읍면동,투표구) tuple —
	// they must NOT be merged. 무효 투표수 carries an interior space.
	csvText := "시도명,선거구명,법정읍면동명,투표구명,후보자,득표수\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,선거인수,2512\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,투표수,2500\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,더불어민주당 곽상언,1400\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,무소속 고주환,1090\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,무효 투표수,10\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,기권자수,12\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,선거인수,655\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,투표수,650\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,국민의힘 황두남,650\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,무효 투표수,0\n" +
		"부산광역시,중구영도구,중앙동,관내사전투표,기권자수,5\n"

	recs, err := ParseResults([]byte(csvText))
	if err != nil {
		t.Fatalf("ParseResults: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2 (same-name booths must not merge)", len(recs))
	}
	a := recs[0]
	if a.Electorate != 2512 || a.Votes != 2500 || a.Invalid != 10 || a.Abstention != 12 {
		t.Errorf("metrics mismapped: %+v", a)
	}
	if len(a.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2", len(a.Candidates))
	}
	if a.Candidates[0].Party != "더불어민주당" || a.Candidates[0].Name != "곽상언" || a.Candidates[0].Votes != 1400 {
		t.Errorf("candidate[0] = %+v", a.Candidates[0])
	}
	if a.Candidates[1].Party != "무소속" || a.Candidates[1].Name != "고주환" {
		t.Errorf("independent split wrong: %+v", a.Candidates[1])
	}
	if recs[1].Electorate != 655 {
		t.Errorf("second unit electorate = %d, want 655", recs[1].Electorate)
	}
}

func TestParseResultsEUCKR(t *testing.T) {
	utf8CSV := "시도명,선거구명,법정읍면동명,투표구명,후보자,득표수\n" +
		"서울특별시,종로구,청운효자동,제1투,선거인수,100\n" +
		"서울특별시,종로구,청운효자동,제1투,국민의힘 최재형,55\n"
	euckr, err := io.ReadAll(transform.NewReader(strings.NewReader(utf8CSV), korean.EUCKR.NewEncoder()))
	if err != nil {
		t.Fatal(err)
	}
	recs, err := ParseResults(euckr)
	if err != nil {
		t.Fatalf("ParseResults(EUC-KR): %v", err)
	}
	if len(recs) != 1 || recs[0].Town != "청운효자동" || recs[0].Candidates[0].Name != "최재형" {
		t.Errorf("EUC-KR decode/parse wrong: %+v", recs)
	}
}

func TestParseResultsRejectsUnknownLayout(t *testing.T) {
	if _, err := ParseResults([]byte("a,b,c\n1,2,3\n")); err == nil {
		t.Error("expected error for non-개표결과 layout")
	}
}

func TestClassifyVoteType(t *testing.T) {
	cases := []struct {
		town, booth, want string
	}{
		{"청운효자동", "제1투", "본투표"},
		{"청운효자동", "관내사전투표", "관내사전"},
		{"관외사전투표", "", "관외사전"},
		{"거소·선상투표", "", "거소선상"},
		{"중앙동", "", "본투표"},
	}
	for _, c := range cases {
		if got := classifyVoteType(c.town, c.booth); got != c.want {
			t.Errorf("classifyVoteType(%q,%q) = %q, want %q", c.town, c.booth, got, c.want)
		}
	}
}

func TestDownload(t *testing.T) {
	c := testClient(t, necServer(t).URL)
	path, err := c.Download(context.Background(), "15025527", t.TempDir())
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if got := filepath.Base(path); got != "중앙선거관리위원회_국회의원선거 개표결과_20240410.csv" {
		t.Errorf("filename = %q", got)
	}
}

func sampleRecs() []ResultRecord {
	return []ResultRecord{
		{Sido: "서울", District: "종로구", Town: "청운효자동", Booth: "제1투", VoteType: "본투표",
			Electorate: 1000, Votes: 800, Invalid: 20, Abstention: 200,
			Candidates: []CandidateVote{{"A당", "김갑", 500}, {"B당", "이을", 280}}},
		{Sido: "서울", District: "종로구", Town: "삼청동", Booth: "제1투", VoteType: "본투표",
			Electorate: 500, Votes: 400, Invalid: 10, Abstention: 100,
			Candidates: []CandidateVote{{"A당", "김갑", 200}, {"B당", "이을", 190}}},
		{Sido: "서울", District: "종로구", Town: "청운효자동", Booth: "관내사전투표", VoteType: "관내사전",
			Electorate: 300, Votes: 290, Invalid: 0, Abstention: 10,
			Candidates: []CandidateVote{{"A당", "김갑", 100}, {"B당", "이을", 190}}},
	}
}

func TestAggregateSgg(t *testing.T) {
	out := Aggregate(sampleRecs(), AggSgg, false)
	if len(out) != 1 {
		t.Fatalf("got %d groups, want 1 선거구", len(out))
	}
	r := out[0]
	if r.Sido != "서울" || r.District != "종로구" || r.Town != "" {
		t.Errorf("dimensions wrong: %+v", r)
	}
	if r.Electorate != 1800 || r.Votes != 1490 || r.Invalid != 30 || r.Abstention != 310 {
		t.Errorf("metric sums wrong: %+v", r)
	}
	if r.ValidVotes != 1460 { // 1490 - 30
		t.Errorf("validVotes = %d, want 1460", r.ValidVotes)
	}
	if r.Turnout < 0.827 || r.Turnout > 0.828 { // 1490/1800
		t.Errorf("turnout = %v, want ~0.8278", r.Turnout)
	}
	if len(r.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2", len(r.Candidates))
	}
	if r.Candidates[0].Party != "A당" || r.Candidates[0].Votes != 800 { // 500+200+100
		t.Errorf("candidate[0] = %+v, want A당 800", r.Candidates[0])
	}
	if s := r.Candidates[0].Share; s < 0.547 || s > 0.549 { // 800/1460
		t.Errorf("share = %v, want ~0.5479", s)
	}
}

func TestAggregateByVoteType(t *testing.T) {
	out := Aggregate(sampleRecs(), AggSgg, true)
	if len(out) != 2 {
		t.Fatalf("got %d groups, want 2 (본투표 + 관내사전)", len(out))
	}
	byType := map[string]AggregatedRecord{}
	for _, r := range out {
		byType[r.VoteType] = r
	}
	if byType["본투표"].Votes != 1200 { // 800+400
		t.Errorf("본투표 votes = %d, want 1200", byType["본투표"].Votes)
	}
	if byType["관내사전"].Votes != 290 {
		t.Errorf("관내사전 votes = %d, want 290", byType["관내사전"].Votes)
	}
}

func TestAggregateSidoDropsCandidates(t *testing.T) {
	out := Aggregate(sampleRecs(), AggSido, false)
	if len(out) != 1 {
		t.Fatalf("got %d groups, want 1 시도", len(out))
	}
	r := out[0]
	if r.Sido != "서울" || r.District != "" {
		t.Errorf("sido dims wrong: %+v", r)
	}
	if len(r.Candidates) != 0 {
		t.Errorf("sido level must drop candidates, got %d", len(r.Candidates))
	}
	if r.Votes != 1490 {
		t.Errorf("metrics still summed: votes = %d, want 1490", r.Votes)
	}
}

// TestAggregateNational verifies that AggNational collapses all records into a
// single group with empty spatial dimensions and no candidates.
// sampleRecs totals: Electorate=1000+500+300=1800, Votes=800+400+290=1490,
// Invalid=20+10+0=30, Abstention=200+100+10=310.
func TestAggregateNational(t *testing.T) {
	out := Aggregate(sampleRecs(), AggNational, false)
	if len(out) != 1 {
		t.Fatalf("got %d groups, want 1 national group", len(out))
	}
	r := out[0]
	// All spatial dimensions must be empty at national level.
	if r.Sido != "" || r.District != "" || r.Town != "" {
		t.Errorf("spatial dims must be empty at national level: Sido=%q District=%q Town=%q", r.Sido, r.District, r.Town)
	}
	// Candidates are not meaningful across different districts — must be empty.
	if len(r.Candidates) != 0 {
		t.Errorf("national level must have no candidates, got %d", len(r.Candidates))
	}
	// Metric sums across all three records in sampleRecs.
	if r.Electorate != 1800 {
		t.Errorf("Electorate = %d, want 1800", r.Electorate)
	}
	if r.Votes != 1490 {
		t.Errorf("Votes = %d, want 1490", r.Votes)
	}
	if r.Invalid != 30 {
		t.Errorf("Invalid = %d, want 30", r.Invalid)
	}
	if r.Abstention != 310 {
		t.Errorf("Abstention = %d, want 310", r.Abstention)
	}
}

// TestAggregateTown verifies that AggTown groups by 읍면동, merging different
// booths within the same town. sampleRecs has two towns:
//   - 청운효자동: records 0 (본투표) + 2 (관내사전) → Electorate=1300, Votes=1090, Invalid=20
//   - 삼청동:    record 1 only → Electorate=500, Votes=400
//
// Candidates must be kept at town level (len > 0).
func TestAggregateTown(t *testing.T) {
	out := Aggregate(sampleRecs(), AggTown, false)
	// Two distinct towns: 청운효자동 and 삼청동.
	if len(out) != 2 {
		t.Fatalf("got %d groups, want 2 town groups", len(out))
	}
	// Build a lookup by town name for order-independent assertions.
	byTown := map[string]AggregatedRecord{}
	for _, r := range out {
		byTown[r.Town] = r
	}
	chung, ok := byTown["청운효자동"]
	if !ok {
		t.Fatal("청운효자동 group missing")
	}
	// Records 0 and 2 merged: Electorate=1000+300=1300, Votes=800+290=1090, Invalid=20+0=20.
	if chung.Electorate != 1300 {
		t.Errorf("청운효자동 Electorate = %d, want 1300", chung.Electorate)
	}
	if chung.Votes != 1090 {
		t.Errorf("청운효자동 Votes = %d, want 1090", chung.Votes)
	}
	if chung.Invalid != 20 {
		t.Errorf("청운효자동 Invalid = %d, want 20", chung.Invalid)
	}
	// Candidates must be populated at town level.
	if len(chung.Candidates) == 0 {
		t.Error("청운효자동 must have candidates at town level")
	}
	// 삼청동 group corresponds to record 1 only.
	sam, ok := byTown["삼청동"]
	if !ok {
		t.Fatal("삼청동 group missing")
	}
	if sam.Electorate != 500 || sam.Votes != 400 {
		t.Errorf("삼청동 metrics wrong: Electorate=%d Votes=%d, want 500/400", sam.Electorate, sam.Votes)
	}
}

func TestDeriveVoteType(t *testing.T) {
	cases := []struct {
		gubun, wantVT string
		wantAgg       bool
	}{
		{"합계", "", true},
		{"소계", "", true},
		{"거소투표", "거소", false},
		{"관외사전투표", "관외사전", false},
		{"관내사전투표", "관내사전", false},
		{"무효 투표수", "본투표", false}, // 공백 포함 임의값도 default
		{"청운효자동", "본투표", false},
	}
	for _, c := range cases {
		vt, agg := deriveVoteType(c.gubun)
		if vt != c.wantVT || agg != c.wantAgg {
			t.Errorf("deriveVoteType(%q) = (%q,%v), want (%q,%v)", c.gubun, vt, agg, c.wantVT, c.wantAgg)
		}
	}
}

func TestElectionResultDim(t *testing.T) {
	e := ElectionResult{Dimensions: []Dimension{{"시도명", "서울"}, {"구분", "소계"}}}
	if e.Dim("시도명") != "서울" {
		t.Errorf("Dim(시도명) = %q", e.Dim("시도명"))
	}
	if e.Dim("없는라벨") != "" {
		t.Errorf("missing label should be empty, got %q", e.Dim("없는라벨"))
	}
}

// TestAggregateZeroElectorateNoNaN verifies that a record with Electorate==0
// and all-zero votes produces Turnout==0 and candidate Share==0 — not NaN —
// since the implementation guards the division by zero.
func TestAggregateZeroElectorateNoNaN(t *testing.T) {
	recs := []ResultRecord{
		{Sido: "서울", District: "종로구", Town: "청운효자동", Booth: "제1투", VoteType: "본투표",
			Electorate: 0, Votes: 0, Invalid: 0, Abstention: 0,
			Candidates: []CandidateVote{{"A당", "김갑", 0}}},
	}
	out := Aggregate(recs, AggSgg, false)
	if len(out) != 1 {
		t.Fatalf("got %d groups, want 1", len(out))
	}
	r := out[0]
	if math.IsNaN(r.Turnout) {
		t.Errorf("Turnout must not be NaN when Electorate==0, got %v", r.Turnout)
	}
	if r.Turnout != 0 {
		t.Errorf("Turnout = %v, want 0", r.Turnout)
	}
	if len(r.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(r.Candidates))
	}
	if math.IsNaN(r.Candidates[0].Share) {
		t.Errorf("candidate Share must not be NaN when ValidVotes==0, got %v", r.Candidates[0].Share)
	}
	if r.Candidates[0].Share != 0 {
		t.Errorf("candidate Share = %v, want 0", r.Candidates[0].Share)
	}
}
