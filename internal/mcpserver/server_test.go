package mcpserver

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
