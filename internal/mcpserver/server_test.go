package mcpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xuri/excelize/v2"
)

// necFixtureServer serves the minimal 3-step download flow
// (fileData.do → selectFileDataDownload.do → fileDownload.do) that
// internal/nec/nec_test.go's TestDownload exercises, but with a minimal
// 개표결과 CSV body so ingest_results has something to normalize.
func necFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	const detailHTML = `<html><body><script>
function init(){ fn_fileDataDown('15025527', 'uddi:abcd-1234-ef', '','1', '3'); }
</script></body></html>`
	const resolveJSON = `{"status":true,"atchFileId":"FILE_0003172714","fileDetailSn":1,
"dataSetFileDetailInfo":{"dataNm":"개표결과"}}`
	csv := "시도명,선거구명,법정읍면동명,투표구명,후보자,득표수\n" +
		"서울,종로구,청운효자동,제1투,선거인수,100\n" +
		"서울,종로구,청운효자동,제1투,투표수,80\n" +
		"서울,종로구,청운효자동,제1투,무효투표수,5\n" +
		"서울,종로구,청운효자동,제1투,기권자수,20\n" +
		"서울,종로구,청운효자동,제1투,A당 김,40\n" +
		"서울,종로구,청운효자동,제1투,B당 이,35\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/15025527/fileData.do":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.Write([]byte(detailHTML))
		case "/tcs/dss/selectFileDataDownload.do":
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.Write([]byte(resolveJSON))
		case "/cmm/cmm/fileDownload.do":
			w.Header().Set("Content-Disposition", `attachment; filename="개표결과.csv"`)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte(csv))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestIngestResultsTool 는 httptest 픽스처에서 다운로드→파싱→적재→질의를 왕복 검증한다.
func TestIngestResultsTool(t *testing.T) {
	fixture := necFixtureServer(t)
	necClient := nec.New(nec.WithBaseURL(fixture.URL), nec.WithDelay(0))

	p := filepath.Join(t.TempDir(), "k.db")
	deps := Deps{DBPath: p, NEC: necClient}
	srv := New(deps)

	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "ingest_results",
		Arguments: map[string]any{"pk": "15025527"},
	})
	if err != nil {
		t.Fatalf("CallTool(ingest_results): %v", err)
	}
	if res.IsError {
		t.Fatalf("ingest_results tool error: %+v", res.Content)
	}

	qres, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query",
		Arguments: map[string]any{"sql": "SELECT count(*) AS n FROM results"},
	})
	if err != nil {
		t.Fatalf("CallTool(query): %v", err)
	}
	if qres.IsError {
		t.Fatalf("query tool error: %+v", qres.Content)
	}

	// Verify directly against the store too, for an unambiguous row count check.
	db, err := store.OpenReadOnly(p)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()
	qr, err := db.Query("SELECT count(*) AS n FROM results", 10)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(qr.Rows) == 0 || len(qr.Rows[0]) == 0 {
		t.Fatal("no rows returned from results count query")
	}
	n, _ := qr.Rows[0][0].(int64)
	if n < 1 {
		t.Errorf("results row count = %v, want >= 1", qr.Rows[0][0])
	}
}

// necSearchFixtureServer serves the data.go.kr dataset-list markup that
// internal/nec/nec_test.go's TestDatasets exercises (two <dl> entries with
// /data/<pk>/fileData.do anchors), so search_datasets has something to parse.
func necSearchFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tcs/dss/selectDataSetList.do":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.Write([]byte(listHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestSearchDatasetsTool 는 selectDataSetList.do 픽스처를 통해 search_datasets
// 왕복(키워드 검색 → dataset 목록)을 검증한다.
func TestSearchDatasetsTool(t *testing.T) {
	fixture := necSearchFixtureServer(t)
	necClient := nec.New(nec.WithBaseURL(fixture.URL), nec.WithDelay(0))

	p := filepath.Join(t.TempDir(), "k.db")
	deps := Deps{DBPath: p, NEC: necClient}
	srv := New(deps)

	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_datasets",
		Arguments: map[string]any{"keyword": "개표결과"},
	})
	if err != nil {
		t.Fatalf("CallTool(search_datasets): %v", err)
	}
	if res.IsError {
		t.Fatalf("search_datasets tool error: %+v", res.Content)
	}

	var out searchOut
	if len(res.Content) == 0 {
		t.Fatal("search_datasets returned no content")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("unmarshal search_datasets result: %v", err)
	}
	if len(out.Datasets) < 1 {
		t.Fatalf("got %d datasets, want >= 1: %+v", len(out.Datasets), out.Datasets)
	}
}

// buildTurnoutZipBytes mirrors internal/nec/turnout_test.go's fixture: a minimal
// 성별·연령대별 투표율 xlsx wrapped in a zip. Kept local to avoid cross-package test deps.
func buildTurnoutZipBytes(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sh := "서울"
	f.SetSheetName("Sheet1", sh)
	s := func(c, v string) { f.SetCellValue(sh, c, v) }
	s("A1", "성별·연령대별 투표율(구시군별)")
	s("A3", "[표본-일반][서울특별시]")
	s("A4", "구분")
	s("D4", "합계")
	s("A5", "전체")
	s("B5", "합계")
	s("C5", "선거인수")
	s("D5", "100")
	s("C6", "투표자수")
	s("D6", "60")
	s("C7", "투표율")
	s("D7", "60.0")
	var xb bytes.Buffer
	f.Write(&xb)
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("02_선거일 투표/성별·연령대별.xlsx")
	w.Write(xb.Bytes())
	zw.Close()
	return zb.Bytes()
}

func TestIngestTurnoutTool(t *testing.T) {
	zipBytes := buildTurnoutZipBytes(t)
	const detailHTML = `<html><body><script>
function init(){ fn_fileDataDown('15143936', 'uddi:aaaa-bbbb-cc', '','1', '3'); }
</script></body></html>`
	const resolveJSON = `{"status":true,"atchFileId":"FILE_0001","fileDetailSn":1,
"dataSetFileDetailInfo":{"dataNm":"투표율분석"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/15143936/fileData.do":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.Write([]byte(detailHTML))
		case "/tcs/dss/selectFileDataDownload.do":
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.Write([]byte(resolveJSON))
		case "/cmm/cmm/fileDownload.do":
			w.Header().Set("Content-Disposition", `attachment; filename="투표율분석.zip"`)
			w.Header().Set("Content-Type", "application/zip")
			w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := filepath.Join(t.TempDir(), "k.db")
	deps := Deps{DBPath: p, NEC: nec.New(nec.WithBaseURL(srv.URL), nec.WithDelay(0))}
	sv := New(deps)
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	sv.Connect(ctx, st, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, _ := client.Connect(ctx, ct, nil)
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "ingest_turnout", Arguments: map[string]any{"pk": "15143936"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	// verify rows landed
	ro, _ := store.OpenReadOnly(p)
	defer ro.Close()
	qr, err := ro.Query("SELECT count(*) FROM turnout", 10)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(qr.Rows) != 1 {
		t.Fatalf("no count row")
	}
}

func TestQueryToolRoundTrip(t *testing.T) {
	// seed a DB
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := store.Open(p)
	db.SQL().Exec(`INSERT INTO datasets(source, ingested_at, row_count) VALUES('nec','now',0)`)
	db.Close()

	srv := New(Deps{DBPath: p})
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query",
		Arguments: map[string]any{"sql": "SELECT count(*) AS n FROM datasets"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
}

func TestSchemaResource(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := store.Open(p)
	db.Close()
	srv := New(Deps{DBPath: p})
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	srv.Connect(ctx, st, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, _ := client.Connect(ctx, ct, nil)
	defer cs.Close()

	rr, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "kvote://schema"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(rr.Contents) == 0 || rr.Contents[0].Text == "" {
		t.Fatal("schema resource empty")
	}
}

// MCP query 는 limit 미지정 시 200행으로 캡된다(에이전트 컨텍스트 보호) —
// CLI(store 기본 1000)와 다른 MCP 전용 기본값.
func TestQueryToolDefaultLimit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := store.Open(p)
	for i := 0; i < 250; i++ {
		db.SQL().Exec(`INSERT INTO datasets(source, ingested_at, row_count) VALUES('nec','now',0)`)
	}
	db.Close()

	srv := New(Deps{DBPath: p})
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	srv.Connect(ctx, st, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, _ := client.Connect(ctx, ct, nil)
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "query", Arguments: map[string]any{"sql": "SELECT id FROM datasets"}})
	if err != nil || res.IsError {
		t.Fatalf("CallTool: err=%v isError=%v", err, res != nil && res.IsError)
	}
	var qr store.QueryResult
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &qr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(qr.Rows) != 200 || !qr.Truncated {
		t.Errorf("rows=%d truncated=%v, want 200/true", len(qr.Rows), qr.Truncated)
	}
}
