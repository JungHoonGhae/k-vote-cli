package mcpserver

import (
	"context"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type searchIn struct {
	Keyword string `json:"keyword" jsonschema:"free-text query for NEC file datasets on data.go.kr"`
}
type searchOut struct {
	Datasets []nec.Dataset `json:"datasets"`
}
type latestIn struct {
	Keyword string `json:"keyword" jsonschema:"one election type keyword, e.g. '지방선거 개표결과'"`
}
type latestOut struct {
	Refs []nec.LatestRef `json:"refs"`
}

// registerSearchTools registers keyless dataset-discovery tools: search_datasets
// (freeform keyword search over data.go.kr file datasets) and list_elections
// (latest-round lookup per source keyed by election-type keyword).
func registerSearchTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_datasets",
		Description: "선관위가 data.go.kr 에 공개한 파일 데이터셋을 키워드로 검색한다. 반환된 publicDataPk 를 ingest_results 에 넘긴다.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, *searchOut, error) {
		ds, err := deps.NEC.Datasets(ctx, nec.SearchOptions{Keyword: in.Keyword})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &searchOut{Datasets: ds}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_elections",
		Description: "선거종류 키워드로 각 소스(data.go.kr·개방포털)의 최신 회차 데이터셋을 찾는다. 키리스.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in latestIn) (*mcp.CallToolResult, *latestOut, error) {
		refs := deps.NEC.LatestDataset(ctx, in.Keyword)
		return nil, &latestOut{Refs: refs}, nil
	})
}
