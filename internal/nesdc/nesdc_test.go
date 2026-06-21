package nesdc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestDownloadURLNotReEncoded(t *testing.T) {
	c := New(WithBaseURL("https://example.test/portal"))
	a := Attachment{AtchFileID: "abc%2Fdef%3D", FileSn: "x%3D%3D", BbsID: "B0000005", BbsKey: "k%3D"}
	want := "https://example.test/portal/cmm/fms/FileDown.do?atchFileId=abc%2Fdef%3D&fileSn=x%3D%3D&bbsId=B0000005&bbsKey=k%3D"
	if got := c.DownloadURL(a); got != want {
		t.Errorf("DownloadURL =\n %q\nwant\n %q", got, want)
	}
}
