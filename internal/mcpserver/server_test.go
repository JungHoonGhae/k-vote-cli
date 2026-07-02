package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
