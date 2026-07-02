// Package mcpserver exposes kvote's data over the Model Context Protocol: 탐색·
// 키리스 수집·read-only SQL 질의를 tool 로 제공한다. 데이터·파생값은 store 계층이
// 정의하고, 이 패키지는 조립만 한다 (판단 없음 — 중립).
package mcpserver

import (
	"context"
	"fmt"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Deps carries the collaborators the server needs.
type Deps struct {
	DBPath string
	NEC    *nec.Client
	NESDC  *nesdc.Client
}

// --- tool I/O types (struct → JSON schema 자동 추론) ---

type queryIn struct {
	SQL   string `json:"sql" jsonschema:"read-only SQL statement to execute against the kvote DB (see kvote://schema)"`
	Limit int    `json:"limit,omitempty" jsonschema:"max rows to return (default 1000)"`
}

// New builds the MCP server with all tools and the schema resource registered.
func New(deps Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "kvote",
		Title:   "kvote — 한국 선거 공개 데이터",
		Version: "0.1.0",
	}, nil)

	// query: read-only SQL passthrough.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "query",
		Description: "kvote 로컬 DB에 read-only SQL을 실행한다. 스키마·파생값 정의는 먼저 kvote://schema 리소스를 읽을 것. 쓰기 SQL은 엔진이 거부한다.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in queryIn) (*mcp.CallToolResult, *store.QueryResult, error) {
		db, err := store.OpenReadOnly(deps.DBPath)
		if err != nil {
			return errResult(fmt.Sprintf("DB 열기 실패: %v — 먼저 ingest_results/ingest_polls 로 적재하세요", err)), nil, nil
		}
		defer db.Close()
		qr, err := db.Query(in.SQL, in.Limit)
		if err != nil {
			return errResult(fmt.Sprintf("질의 오류: %v", err)), nil, nil
		}
		return nil, qr, nil
	})

	registerIngestTools(s, deps) // Task 8
	registerSearchTools(s, deps) // Task 9

	// schema resource.
	s.AddResource(&mcp.Resource{
		Name:        "schema",
		URI:         "kvote://schema",
		MIMEType:    "text/markdown",
		Description: "kvote DB 테이블·뷰 스키마와 표준 파생값 정의. query 전에 읽으세요.",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      "kvote://schema",
			MIMEType: "text/markdown",
			Text:     store.SchemaDoc,
		}}}, nil
	})

	return s
}

// errResult wraps a user-facing error message as a non-fatal tool error result.
func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

// Serve runs the server over stdio until the client disconnects or ctx is cancelled.
func Serve(ctx context.Context, deps Deps) error {
	return New(deps).Run(ctx, &mcp.StdioTransport{})
}

// registerSearchTools is implemented in search.go (Task 9).
// registerIngestTools is implemented in ingest.go (Task 8).
