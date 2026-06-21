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

	"github.com/xuri/excelize/v2"
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

func buildXLSX(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	// 시트1: 후보형 (시도지사 모사) — 차원 4열: 선거구명,구시군명,읍면동명,구분
	s1 := "시·도지사"
	f.SetSheetName(f.GetSheetName(0), s1)
	put := func(sheet string, rows [][]any) {
		for i, row := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if err := f.SetSheetRow(sheet, cell, &row); err != nil {
				t.Fatal(err)
			}
		}
	}
	put(s1, [][]any{
		{"선거구명", "구시군명", "읍면동명", "구분", "선거인수", "투표수", "후보자별 득표수", "", "계", "무효투표수", "기권수"},
		{"", "", "", "", "선거인수", "선거인수", "후보1", "후보2", "", "", ""},
		{"서울특별시", "종로구", "", "", "", "", "더불어민주당\n송영길", "국민의힘\n오세훈", "", "", ""},
		{"서울특별시", "종로구", "", "합계", "1,000", "700", "300", "390", "690", "10", "300"},
		{"서울특별시", "종로구", "", "관외사전투표", "100", "100", "44", "54", "98", "2", "0"},
		{"서울특별시", "종로구", "청운효자동", "소계", "500", "400", "180", "210", "390", "10", "100"},
		{"서울특별시", "종로구", "청운효자동", "관내사전투표", "200", "200", "90", "108", "198", "2", "0"},
	})
	// 시트2: 비례형 — 차원 3열, 정당만(줄바꿈 없음)
	s2 := "광역의원비례대표"
	f.NewSheet(s2)
	put(s2, [][]any{
		{"시도명", "구시군명", "읍면동명", "구분", "선거인수", "투표수", "후보자별 득표수", "", "계", "무효투표수", "기권수"},
		{"", "", "", "", "선거인수", "선거인수", "정당1", "정당2", "", "", ""},
		{"서울특별시", "종로구", "", "", "", "", "더불어민주당", "국민의힘", "", "", ""},
		{"서울특별시", "종로구", "", "합계", "1,000", "700", "320", "360", "680", "20", "300"},
	})
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseResultsXLSX(t *testing.T) {
	recs, err := ParseResultsXLSX(buildXLSX(t))
	if err != nil {
		t.Fatalf("ParseResultsXLSX: %v", err)
	}
	// 시트1 데이터행 4개(합계/관외사전/소계/관내사전) + 시트2 1개(합계) = 5; 후보정의행은 제외
	if len(recs) != 5 {
		t.Fatalf("got %d records, want 5: %+v", len(recs), recs)
	}
	// 첫 레코드: 시도지사 합계
	r := recs[0]
	if r.Race != "시·도지사" {
		t.Errorf("race = %q", r.Race)
	}
	if r.Dim("선거구명") != "서울특별시" || r.Dim("구시군명") != "종로구" || r.Dim("구분") != "합계" {
		t.Errorf("dimensions wrong: %+v", r.Dimensions)
	}
	if !r.Aggregate || r.VoteType != "" {
		t.Errorf("합계 should be aggregate w/ empty voteType: agg=%v vt=%q", r.Aggregate, r.VoteType)
	}
	if r.Electorate != 1000 || r.Votes != 700 || r.Invalid != 10 || r.Abstention != 300 {
		t.Errorf("metrics wrong: %+v", r)
	}
	if len(r.Candidates) != 2 || r.Candidates[0].Party != "더불어민주당" || r.Candidates[0].Name != "송영길" || r.Candidates[0].Votes != 300 {
		t.Errorf("candidates wrong: %+v", r.Candidates)
	}
	// 관내사전투표 leaf
	var leaf *ElectionResult
	for i := range recs {
		if recs[i].Dim("구분") == "관내사전투표" && recs[i].Race == "시·도지사" {
			leaf = &recs[i]
		}
	}
	if leaf == nil || leaf.VoteType != "관내사전" || leaf.Aggregate {
		t.Fatalf("관내사전 leaf wrong: %+v", leaf)
	}
	// 비례: 정당만, Name 빈칸
	var prop *ElectionResult
	for i := range recs {
		if recs[i].Race == "광역의원비례대표" {
			prop = &recs[i]
		}
	}
	if prop == nil || len(prop.Candidates) != 2 || prop.Candidates[0].Party != "더불어민주당" || prop.Candidates[0].Name != "" {
		t.Fatalf("비례 candidate wrong: %+v", prop)
	}
}

// buildXLSXGyo builds a minimal 교육감 XLSX:
//   - Sheet name: "교육감"
//   - row0: dimension headers + 선거인수, 투표수, 후보자별 득표수, "", 계, 무효투표수, 기권수
//   - row1: merged-cell remnant (all empty)
//   - row2: candidate-definition row — single-part names, dims empty, 선거인수 empty
//   - row3: data row (구분=합계, with numbers)
func buildXLSXGyo(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := "교육감"
	f.SetSheetName(f.GetSheetName(0), sheet)
	put := func(rows [][]any) {
		for i, row := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if err := f.SetSheetRow(sheet, cell, &row); err != nil {
				t.Fatal(err)
			}
		}
	}
	put([][]any{
		// row0: header labels
		{"선거구명", "구시군명", "읍면동명", "구분", "선거인수", "투표수", "후보자별 득표수", "", "계", "무효투표수", "기권수"},
		// row1: merged remnant — all empty
		{"", "", "", "", "", "", "", "", "", "", ""},
		// row2: candidate-definition row — single-part person names, 선거인수 empty
		{"", "", "", "", "", "", "최보선", "조희연", "", "", ""},
		// row3: data row
		{"서울특별시", "강남구", "", "합계", "500000", "350000", "180000", "165000", "345000", "5000", "150000"},
	})
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestParseResultsXLSXGyo verifies that single-part candidate cells on a
// 교육감 sheet map to Name (not Party), since 교육감 races are non-partisan.
func TestParseResultsXLSXGyo(t *testing.T) {
	recs, err := ParseResultsXLSX(buildXLSXGyo(t))
	if err != nil {
		t.Fatalf("ParseResultsXLSX(교육감): %v", err)
	}
	// One data row (the 합계 row).
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1: %+v", len(recs), recs)
	}
	r := recs[0]
	if r.Race != "교육감" {
		t.Errorf("race = %q, want 교육감", r.Race)
	}
	if r.Electorate != 500000 {
		t.Errorf("electorate = %d, want 500000", r.Electorate)
	}
	if len(r.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(r.Candidates), r.Candidates)
	}
	// 교육감: single-part → Name set, Party empty.
	if r.Candidates[0].Party != "" || r.Candidates[0].Name != "최보선" {
		t.Errorf("candidates[0] = %+v, want Party=\"\" Name=\"최보선\"", r.Candidates[0])
	}
	if r.Candidates[1].Party != "" || r.Candidates[1].Name != "조희연" {
		t.Errorf("candidates[1] = %+v, want Party=\"\" Name=\"조희연\"", r.Candidates[1])
	}
}

func TestResultRecordToElectionResult(t *testing.T) {
	r := ResultRecord{
		Sido: "서울", District: "종로구", Town: "청운효자동", Booth: "관내사전투표",
		VoteType: "관내사전", Electorate: 100, Votes: 90, Invalid: 2, Abstention: 10,
		Candidates: []CandidateVote{{"A당", "김갑", 50}},
	}
	e := r.ToElectionResult()
	if e.Dim("시도명") != "서울" || e.Dim("선거구명") != "종로구" || e.Dim("법정읍면동명") != "청운효자동" || e.Dim("투표구명") != "관내사전투표" {
		t.Errorf("dimensions wrong: %+v", e.Dimensions)
	}
	if e.VoteType != "관내사전" || e.Aggregate {
		t.Errorf("voteType/aggregate wrong: %q %v", e.VoteType, e.Aggregate)
	}
	if e.Electorate != 100 || len(e.Candidates) != 1 || e.Candidates[0].Name != "김갑" {
		t.Errorf("fields wrong: %+v", e)
	}
}

// TestResultRecordToElectionResultGyoSeonSang verifies Fix B: CSV 거소선상
// voteType is normalized to 거소 in ToElectionResult so the common schema
// matches the XLSX vocabulary (거소투표 → 거소).
func TestResultRecordToElectionResultGyoSeonSang(t *testing.T) {
	r := ResultRecord{
		Sido: "서울", District: "종로구", Town: "거소·선상투표", Booth: "",
		VoteType: "거소선상", Electorate: 50, Votes: 40, Invalid: 0, Abstention: 10,
		Candidates: []CandidateVote{{"A당", "홍길동", 40}},
	}
	e := r.ToElectionResult()
	if e.VoteType != "거소" {
		t.Errorf("ToElectionResult: VoteType = %q, want \"거소\" (CSV 거소선상→거소 정규화)", e.VoteType)
	}
	// Raw ResultRecord.VoteType must remain unchanged (classifyVoteType P1 stays).
	if r.VoteType != "거소선상" {
		t.Errorf("ResultRecord.VoteType mutated: got %q, want \"거소선상\"", r.VoteType)
	}
}

func TestParseResultsXLSXSkipsUnanchored(t *testing.T) {
	f := excelize.NewFile()
	f.SetSheetName(f.GetSheetName(0), "엉뚱시트")
	cell, _ := excelize.CoordinatesToCellName(1, 1)
	row := []any{"아무거나", "헤더", "여기"}
	f.SetSheetRow("엉뚱시트", cell, &row)
	f.NewSheet("시·도지사")
	good := [][]any{
		{"선거구명", "구시군명", "읍면동명", "구분", "선거인수", "투표수", "후보자별 득표수", "계", "무효투표수", "기권수"},
		{"", "", "", "", "", "", "후보1", "", "", ""},
		{"서울특별시", "종로구", "", "", "", "", "무소속\n홍길동", "", "", ""},
		{"서울특별시", "종로구", "", "합계", "10", "8", "8", "8", "0", "2"},
	}
	for i, r := range good {
		c, _ := excelize.CoordinatesToCellName(1, i+1)
		f.SetSheetRow("시·도지사", c, &r)
	}
	buf, _ := f.WriteToBuffer()
	recs, err := ParseResultsXLSX(buf.Bytes())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(recs) != 1 || recs[0].Race != "시·도지사" {
		t.Errorf("unanchored sheet should be skipped, valid one parsed: %+v", recs)
	}
}

// TestParseResultsXLSXCandidateRedefinition verifies the load-bearing
// "candidate header refreshes per 선거구" behavior: each 선거구 block carries
// its OWN candidate-definition row, so a later block's data rows must use the
// candidates declared in THAT block's header, not the first block's header.
func TestParseResultsXLSXCandidateRedefinition(t *testing.T) {
	t.Helper()
	f := excelize.NewFile()
	sheet := "시·도의회의원"
	f.SetSheetName(f.GetSheetName(0), sheet)

	put := func(rows [][]any) {
		for i, row := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if err := f.SetSheetRow(sheet, cell, &row); err != nil {
				t.Fatal(err)
			}
		}
	}

	// row0: anchor labels. Dimension cols: 읍면동명(0), 구분(1). Then 선거인수, 투표수,
	// 후보자별 득표수(start), 계(end), 무효투표수, 기권수.
	// row1: merged-cell remnant (all empty).
	// row2: Block-1 candidate-definition row (읍면동 empty, 구분 empty, 선거인수 empty).
	// row3: Block-1 data row (구분=합계, with numbers).
	// row4: Block-2 candidate-definition row with DIFFERENT candidates.
	// row5: Block-2 data row.
	put([][]any{
		// row0: headers
		{"읍면동명", "구분", "선거인수", "투표수", "후보자별 득표수", "", "계", "무효투표수", "기권수"},
		// row1: merged remnant
		{"", "", "", "", "", "", "", "", ""},
		// row2: Block-1 candidate-def (선거인수 empty = discriminator)
		{"", "", "", "", "더불어민주당\n채행숙", "국민의힘\n윤종복", "", "", ""},
		// row3: Block-1 data
		{"", "합계", "500", "400", "220", "170", "390", "10", "100"},
		// row4: Block-2 candidate-def with DIFFERENT candidates
		{"", "", "", "", "더불어민주당\n여봉무", "국민의힘\n김아무개", "", "", ""},
		// row5: Block-2 data
		{"", "합계", "600", "480", "260", "210", "470", "10", "120"},
	})

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	recs, err := ParseResultsXLSX(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseResultsXLSX: %v", err)
	}
	// Two data rows (one per block); two candidate-def rows are skipped.
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(recs), recs)
	}

	// Block-1 record: candidates must be 채행숙 / 윤종복.
	r1 := recs[0]
	if len(r1.Candidates) != 2 {
		t.Fatalf("block-1: got %d candidates, want 2: %+v", len(r1.Candidates), r1.Candidates)
	}
	if r1.Candidates[0].Name != "채행숙" || r1.Candidates[0].Party != "더불어민주당" {
		t.Errorf("block-1 cand[0] = %+v, want 더불어민주당/채행숙", r1.Candidates[0])
	}
	if r1.Candidates[1].Name != "윤종복" || r1.Candidates[1].Party != "국민의힘" {
		t.Errorf("block-1 cand[1] = %+v, want 국민의힘/윤종복", r1.Candidates[1])
	}

	// Block-2 record: candidates must be 여봉무 / 김아무개 (header refreshed).
	r2 := recs[1]
	if len(r2.Candidates) != 2 {
		t.Fatalf("block-2: got %d candidates, want 2: %+v", len(r2.Candidates), r2.Candidates)
	}
	if r2.Candidates[0].Name != "여봉무" || r2.Candidates[0].Party != "더불어민주당" {
		t.Errorf("block-2 cand[0] = %+v, want 더불어민주당/여봉무", r2.Candidates[0])
	}
	if r2.Candidates[1].Name != "김아무개" || r2.Candidates[1].Party != "국민의힘" {
		t.Errorf("block-2 cand[1] = %+v, want 국민의힘/김아무개", r2.Candidates[1])
	}
}

// TestParseResultsXLSXLeafSum verifies that records with Aggregate==false form a
// clean disjoint partition: their per-candidate vote sums match the 합계 row.
// Layout (one 선거구, 2 candidates A and B):
//
//	관외사전투표 (구분):       A=10, B=20  — leaf
//	거소투표 (구분):           A=1,  B=2   — leaf
//	청운효자동/소계 (구분):    A=30, B=40  — aggregate (excluded from leaf sum)
//	청운효자동/관내사전투표:   A=5,  B=8   — leaf
//	청운효자동/제1투 (구분):   A=25, B=32  — leaf (본투표)
//	합계 (구분):               A=41, B=62  — aggregate (the reference total)
//
// Leaf sum: A = 10+1+5+25 = 41, B = 20+2+8+32 = 62. Matches 합계.
func TestParseResultsXLSXLeafSum(t *testing.T) {
	t.Helper()
	f := excelize.NewFile()
	sheet := "시·도의회의원"
	f.SetSheetName(f.GetSheetName(0), sheet)

	put := func(rows [][]any) {
		for i, row := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if err := f.SetSheetRow(sheet, cell, &row); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Columns: 읍면동명(0), 구분(1), 선거인수(2), 투표수(3),
	//          후보자별 득표수 start(4), cand-B(5), 계(6), 무효투표수(7), 기권수(8)
	put([][]any{
		// row0: anchor labels
		{"읍면동명", "구분", "선거인수", "투표수", "후보자별 득표수", "", "계", "무효투표수", "기권수"},
		// row1: merged-cell remnant
		{"", "", "", "", "", "", "", "", ""},
		// row2: candidate-definition row
		{"", "", "", "", "더불어민주당\n후보A", "국민의힘\n후보B", "", "", ""},
		// Data rows:
		// 관외사전투표 leaf
		{"", "관외사전투표", "1000", "30", "10", "20", "30", "0", "0"},
		// 거소투표 leaf
		{"", "거소투표", "0", "3", "1", "2", "3", "0", "0"},
		// 청운효자동 소계 — aggregate
		{"청운효자동", "소계", "500", "65", "30", "40", "70", "5", "0"},
		// 청운효자동 관내사전투표 leaf
		{"청운효자동", "관내사전투표", "200", "13", "5", "8", "13", "0", "0"},
		// 청운효자동 제1투 leaf (본투표)
		{"청운효자동", "제1투", "300", "57", "25", "32", "57", "0", "0"},
		// 합계 — aggregate, reference total
		{"", "합계", "1500", "98", "41", "62", "103", "5", "0"},
	})

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}

	recs, err := ParseResultsXLSX(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseResultsXLSX: %v", err)
	}

	// Find the 합계 row for reference totals.
	var haptaeA, haptaeB int
	var haptaeFound bool
	for _, r := range recs {
		if r.Aggregate && r.Dim("구분") == "합계" {
			haptaeFound = true
			for _, c := range r.Candidates {
				switch c.Name {
				case "후보A":
					haptaeA = c.Votes
				case "후보B":
					haptaeB = c.Votes
				}
			}
		}
	}
	if !haptaeFound {
		t.Fatal("합계 row not found in parsed records")
	}
	if haptaeA != 41 || haptaeB != 62 {
		t.Errorf("합계 row: A=%d B=%d, want A=41 B=62", haptaeA, haptaeB)
	}

	// Sum votes over all leaf records (Aggregate==false).
	leafSumA, leafSumB := 0, 0
	for _, r := range recs {
		if r.Aggregate {
			continue
		}
		for _, c := range r.Candidates {
			switch c.Name {
			case "후보A":
				leafSumA += c.Votes
			case "후보B":
				leafSumB += c.Votes
			}
		}
	}

	if leafSumA != haptaeA {
		t.Errorf("leaf sum A = %d, want %d (= 합계 row)", leafSumA, haptaeA)
	}
	if leafSumB != haptaeB {
		t.Errorf("leaf sum B = %d, want %d (= 합계 row)", leafSumB, haptaeB)
	}
}
