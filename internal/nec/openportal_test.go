package nec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// The catalog links carry ";jsessionid=…" between path and query on cookieless
// requests — the fixtures include it so the parser's tolerance is locked in.
const opCatalogHTML = `<html><body>
<div class="data-card">
  <a href="/open-data/file.do;jsessionid=ABC123?dataId=18" title="중앙선거관리위원회_제8회 전국동시지방선거 개표결과">
    <img src="x.png"/>
  </a>
</div>
<div class="data-card">
  <a href="/open-data/file.do;jsessionid=ABC123?dataId=8" title="중앙선거관리위원회_대통령선거 개표결과"></a>
</div>
<div><a href="/open-data/api.do?dataId=99">API (무시 대상)</a></div>
</body></html>`

const opDetailHTML = `<html><body>
<h3>중앙선거관리위원회_대통령선거 개표결과</h3>
<div>
  <a target="_blank" href="/file-download.do?attachFileId=66" title="제21대 대통령선거 개표결과.xlsx 다운로드">다운로드</a>
  <a target="_blank" href="/file-download.do;jsessionid=ABC123?attachFileId=6" title="제20대 대통령선거 개표결과.xlsx 다운로드">다운로드</a>
</div>
</body></html>`

func opServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-data/data-list.do":
			w.Write([]byte(opCatalogHTML))
		case "/open-data/file.do":
			w.Write([]byte(opDetailHTML))
		case "/file-download.do":
			w.Header().Set("Content-Disposition", `attachment; filename="%EC%A0%9C21%EB%8C%80%20%EB%8C%80%ED%86%B5%EB%A0%B9%EC%84%A0%EA%B1%B0%20%EA%B0%9C%ED%91%9C%EA%B2%B0%EA%B3%BC.xlsx"`)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("PK\x03\x04 fake xlsx bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func opClient(t *testing.T, base string) *Client {
	return New(WithOpenPortalBaseURL(base), WithDelay(0), WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))
}

func TestOpenPortalDatasets(t *testing.T) {
	c := opClient(t, opServer(t).URL)
	ds, err := c.OpenPortalDatasets(context.Background(), "지방선거")
	if err != nil {
		t.Fatalf("OpenPortalDatasets: %v", err)
	}
	// 2 file.do datasets; the api.do link is ignored.
	if len(ds) != 2 {
		t.Fatalf("got %d datasets, want 2: %+v", len(ds), ds)
	}
	if ds[0].DataID != "18" || ds[0].Title != "중앙선거관리위원회_제8회 전국동시지방선거 개표결과" {
		t.Errorf("dataset[0] = %+v (title must come from the title attr, not link text)", ds[0])
	}
	if ds[1].DataID != "8" {
		t.Errorf("dataset[1] dataId = %q, want 8", ds[1].DataID)
	}
}

func TestOpenPortalFiles(t *testing.T) {
	c := opClient(t, opServer(t).URL)
	fs, err := c.OpenPortalFiles(context.Background(), "8")
	if err != nil {
		t.Fatalf("OpenPortalFiles: %v", err)
	}
	if len(fs) != 2 {
		t.Fatalf("got %d files, want 2: %+v", len(fs), fs)
	}
	if fs[0].AttachFileID != "66" || fs[0].Name != "제21대 대통령선거 개표결과.xlsx" {
		t.Errorf("file[0] = %+v (name = title attr minus ' 다운로드')", fs[0])
	}
	// jsessionid in the second link must not break attachFileId extraction.
	if fs[1].AttachFileID != "6" {
		t.Errorf("file[1] attachFileId = %q, want 6 (jsessionid-tolerant)", fs[1].AttachFileID)
	}
	got, ok := PickXLSX(fs)
	if !ok || got.AttachFileID != "66" {
		t.Errorf("PickXLSX = %+v,%v want attachFileId 66", got, ok)
	}
}

func TestOpenPortalDownload(t *testing.T) {
	c := opClient(t, opServer(t).URL)
	path, err := c.OpenPortalDownload(context.Background(), "66", t.TempDir())
	if err != nil {
		t.Fatalf("OpenPortalDownload: %v", err)
	}
	if got := filepath.Base(path); got != "제21대 대통령선거 개표결과.xlsx" {
		t.Errorf("filename = %q", got)
	}
}
