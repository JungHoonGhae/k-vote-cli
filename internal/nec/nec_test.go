package nec

import (
	"context"
	"io"
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
