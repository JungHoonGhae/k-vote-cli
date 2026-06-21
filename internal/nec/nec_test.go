package nec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
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
