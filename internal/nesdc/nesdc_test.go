package nesdc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/xuri/excelize/v2"
)

// fixtureServer serves saved portal HTML for the routes the client hits, so the
// parsers can be exercised end-to-end without touching the network.
func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var fixture string
		switch r.URL.Path {
		case "/bbs/B0000005/list.do":
			fixture = "results_list.html"
		case "/bbs/B0000005/view.do":
			fixture = "results_view.html"
		case "/content/onvy/list.do":
			fixture = "agency_list.html"
		default:
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testClient(t *testing.T, base string) *Client {
	return New(WithBaseURL(base), WithDelay(0), WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))
}

func TestListResults(t *testing.T) {
	srv := fixtureServer(t)
	c := testClient(t, srv.URL)
	board, _ := BoardByName("results")

	res, err := c.List(context.Background(), board, ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("expected list items, got none")
	}
	want := []string{"등록번호", "조사기관명", "조사의뢰자", "조사방법", "표본 추출틀", "여론조사 명칭(지역)", "등록일", "시·도"}
	if len(res.Columns) != len(want) {
		t.Fatalf("columns = %v, want %v", res.Columns, want)
	}
	first := res.Items[0]
	if first.NttID == "" {
		t.Error("first item has empty nttId")
	}
	if first.Values["조사기관명"] == "" {
		t.Errorf("first item missing 조사기관명: %v", first.Values)
	}
	if first.Values["조사방법"] == "" {
		t.Errorf("first item missing 조사방법: %v", first.Values)
	}
}

func TestDetailResults(t *testing.T) {
	srv := fixtureServer(t)
	c := testClient(t, srv.URL)
	board, _ := BoardByName("results")

	d, err := c.Detail(context.Background(), board, "19366")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if d.Title == "" {
		t.Error("expected a non-empty title")
	}
	if len(d.Fields) == 0 {
		t.Fatal("expected metadata fields")
	}
	if d.Summary["조사기관명"] == "" {
		t.Errorf("summary missing 조사기관명: %v", d.Summary)
	}
	if d.Summary["표본오차"] == "" {
		t.Errorf("summary missing 표본오차: %v", d.Summary)
	}
	if len(d.Attachments) == 0 {
		t.Fatal("expected at least one attachment")
	}
	a := d.Attachments[0]
	if a.AtchFileID == "" || a.FileSn == "" {
		t.Errorf("attachment missing download params: %+v", a)
	}
	if a.Name == "" {
		t.Errorf("attachment missing name: %+v", a)
	}
}

func TestAgencies(t *testing.T) {
	srv := fixtureServer(t)
	c := testClient(t, srv.URL)

	res, err := c.Agencies(context.Background(), false, ListOptions{})
	if err != nil {
		t.Fatalf("Agencies: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("expected agency rows")
	}
	if res.Items[0].InsttNum == "" {
		t.Error("first agency missing insttNum")
	}
}

func TestDecodeFilename(t *testing.T) {
	cases := map[string]string{
		// percent-encoded UTF-8 with '+' for spaces
		`attachment; filename="%EC%97%AC%EB%A1%A0+%EC%A1%B0%EC%82%AC.pdf"`: "여론 조사.pdf",
		// raw UTF-8 bytes mis-read as Latin-1 ("리얼.pdf")
		"attachment; filename=\"\xeb\xa6\xac\xec\x96\xbc.pdf\"": "리얼.pdf",
	}
	for cd, want := range cases {
		if got := filenameFromCD(cd); got != want {
			t.Errorf("filenameFromCD(%q) = %q, want %q", cd, got, want)
		}
	}
}

func TestElections(t *testing.T) {
	srv := fixtureServer(t)
	c := testClient(t, srv.URL)

	els, err := c.Elections(context.Background())
	if err != nil {
		t.Fatalf("Elections: %v", err)
	}
	if len(els) == 0 {
		t.Fatal("expected election options")
	}
	for _, e := range els {
		if e.Code == "" || e.Name == "" {
			t.Errorf("incomplete election: %+v", e)
		}
		if strings.Contains(e.Name, "선거구분") {
			t.Errorf("placeholder option leaked: %+v", e)
		}
	}
}

// TestListDateFilterParams locks in the fix for the silently-ignored date
// filter: sdate/edate must be accompanied by searchTime or the portal drops them.
func TestListDateFilterParams(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		b, _ := os.ReadFile(filepath.Join("testdata", "results_list.html"))
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	c := testClient(t, srv.URL)
	board, _ := BoardByName("results")

	if _, err := c.List(context.Background(), board, ListOptions{From: "2025-01-01", To: "2025-01-05"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Get("searchTime") != "1" {
		t.Errorf("searchTime = %q, want default 1 when a range is set", got.Get("searchTime"))
	}
	if got.Get("sdate") != "2025-01-01" || got.Get("edate") != "2025-01-05" {
		t.Errorf("sdate/edate = %q/%q", got.Get("sdate"), got.Get("edate"))
	}

	if _, err := c.List(context.Background(), board, ListOptions{SearchTime: "3", From: "2025-01-01"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Get("searchTime") != "3" {
		t.Errorf("explicit searchTime not honoured: %q", got.Get("searchTime"))
	}
}

func TestResolveFilters(t *testing.T) {
	if got := ResolveSearchField("agency"); got != "1" {
		t.Errorf("ResolveSearchField(agency) = %q, want 1", got)
	}
	if got := ResolveDateField("surveyed"); got != "3" {
		t.Errorf("ResolveDateField(surveyed) = %q, want 3", got)
	}
	// raw codes and unknown values pass through unchanged
	if got := ResolveSearchField("11"); got != "11" {
		t.Errorf("raw code mangled: %q", got)
	}
	if got := ResolveDateField(""); got != "" {
		t.Errorf("empty should stay empty: %q", got)
	}
}

// TestParseAttachmentsHrefForm covers the data/notice board markup where files
// are linked directly via FileDown.do (no onclick view() call), with the
// already-encoded params preserved verbatim.
func TestParseAttachmentsHrefForm(t *testing.T) {
	html := `<html><body>
		<a href="javascript:void(0);" onclick="view('ID%2F1','SN%3D%3D','B0000005','KEY%3D')">설문지.pdf</a>
		<a href="/portal/cmm/fms/FileDown.do?atchFileId=ABC%2Fxyz%3D&fileSn=Q%3D%3D&bbsId=B0000025">누적.xlsx</a>
	</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	atts := parseAttachments(doc)
	if len(atts) != 2 {
		t.Fatalf("got %d attachments, want 2: %+v", len(atts), atts)
	}
	href := atts[1]
	if href.AtchFileID != "ABC%2Fxyz%3D" || href.FileSn != "Q%3D%3D" || href.BbsID != "B0000025" {
		t.Errorf("href attachment params decoded/lost: %+v", href)
	}
	if href.BbsKey != "" {
		t.Errorf("data board has no bbsKey, got %q", href.BbsKey)
	}
}

func TestParseBulkXlsx(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bulk.xlsx")
	f := excelize.NewFile()
	const sheet = "정당지지도(test)"
	f.SetSheetName(f.GetSheetName(0), sheet)
	// two-row header: metadata labels then party names under 정당지지율(%)
	header := []any{"등록번호", "조사기관", "의뢰자", "조사일자", "조사방법", "표본추출틀", "표본수(명)", "접촉률(%)", "응답률(%)", "95%신뢰수준\n표본오차(%p)", "정당지지율(%)", "", ""}
	parties := []any{"", "", "", "", "", "", "", "", "", "", "더불어민주당", "국민의힘", "기타정당"}
	data := []any{"12345", "한국갤럽", "한겨레", "26.01.01.~02.", "무선ARS(100)", "무선RDD", "1000", "20.0", "5.0", "±3.1", "40.0", "35.0", "5.0"}
	for i, row := range [][]any{header, parties, data} {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	recs, err := ParseBulkXlsx(path)
	if err != nil {
		t.Fatalf("ParseBulkXlsx: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	r := recs[0]
	if r.RegNo != "12345" || r.Agency != "한국갤럽" || r.SampleSize != "1000" {
		t.Errorf("metadata mismapped: %+v", r)
	}
	if r.MarginError != "±3.1" {
		t.Errorf("marginError = %q, want ±3.1", r.MarginError)
	}
	if r.Period != sheet {
		t.Errorf("period = %q, want %q", r.Period, sheet)
	}
	want := map[string]string{"더불어민주당": "40.0", "국민의힘": "35.0", "기타정당": "5.0"}
	for k, v := range want {
		if r.PartySupport[k] != v {
			t.Errorf("partySupport[%s] = %q, want %q (full: %v)", k, r.PartySupport[k], v, r.PartySupport)
		}
	}
}

func TestDownloadURLNotReEncoded(t *testing.T) {
	c := New(WithBaseURL("https://example.test/portal"))
	a := Attachment{AtchFileID: "abc%2Fdef%3D", FileSn: "x%3D%3D", BbsID: "B0000005", BbsKey: "k%3D"}
	want := "https://example.test/portal/cmm/fms/FileDown.do?atchFileId=abc%2Fdef%3D&fileSn=x%3D%3D&bbsId=B0000005&bbsKey=k%3D"
	if got := c.DownloadURL(a); got != want {
		t.Errorf("DownloadURL =\n %q\nwant\n %q", got, want)
	}
}

func sampleCompFields() []Field {
	return []Field{
		{Labels: []string{"표본의 크기"}},
		{Labels: []string{"구분", "조사완료 사례수(명)", "가중값 적용 기준 사례수(명)"}},
		{Labels: []string{"전체"}, Values: []string{"1,001", "1001"}},
		{Labels: []string{"성별", "남"}, Values: []string{"546", "496"}},
		{Labels: []string{"여"}, Values: []string{"455", "505"}},
		{Labels: []string{"연령대별", "18~29세"}, Values: []string{"128", "149"}},
		{Labels: []string{"70세 이상"}, Values: []string{"153", "162"}},
		{Labels: []string{"지역별", "서울"}, Values: []string{"198", "185"}},
		{Labels: []string{"제주"}, Values: []string{"14", "12"}},
		{Labels: []string{"조사방법1"}, Values: []string{"무선 ARS"}}, // 블록 종료 트리거
		{Labels: []string{"기본가중", "산출방법"}, Values: []string{"성별·연령별·지역별 가중값 부여"}},
		{Labels: []string{"표본오차"}, Values: []string{"95% 신뢰수준에 ±3.1%P"}},
	}
}

func TestSampleCompositionOf(t *testing.T) {
	sc := SampleCompositionOf(&Detail{Fields: sampleCompFields()})
	if sc == nil {
		t.Fatal("expected a SampleComposition, got nil")
	}
	if sc.Total == nil || sc.Total.Completed != 1001 || sc.Total.Weighted != 1001 {
		t.Errorf("total wrong: %+v", sc.Total)
	}
	if len(sc.Crosstabs) != 3 {
		t.Fatalf("got %d crosstabs, want 3 (성별/연령대별/지역별): %+v", len(sc.Crosstabs), sc.Crosstabs)
	}
	g := sc.Crosstabs[0]
	if g.Dimension != "성별" || len(g.Cells) != 2 || g.Cells[0].Category != "남" || g.Cells[0].Completed != 546 || g.Cells[0].Weighted != 496 {
		t.Errorf("성별 crosstab wrong: %+v", g)
	}
	if g.Cells[1].Category != "여" || g.Cells[1].Completed != 455 {
		t.Errorf("성별 여 wrong: %+v", g.Cells[1])
	}
	if sc.Crosstabs[1].Dimension != "연령대별" || sc.Crosstabs[1].Cells[0].Category != "18~29세" {
		t.Errorf("연령 crosstab wrong: %+v", sc.Crosstabs[1])
	}
	if sc.Crosstabs[2].Dimension != "지역별" || sc.Crosstabs[2].Cells[1].Category != "제주" {
		t.Errorf("지역 crosstab wrong: %+v", sc.Crosstabs[2])
	}
	if sc.Weighting == "" || sc.MarginError != "95% 신뢰수준에 ±3.1%P" {
		t.Errorf("weighting/marginError wrong: %q / %q", sc.Weighting, sc.MarginError)
	}
}

func TestSampleCompositionOfNoBlock(t *testing.T) {
	d := &Detail{Fields: []Field{{Labels: []string{"조사기관명"}, Values: []string{"리얼미터"}}}}
	if sc := SampleCompositionOf(d); sc != nil {
		t.Errorf("expected nil when no composition block, got %+v", sc)
	}
}
