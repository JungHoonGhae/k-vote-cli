package mcpserver

import (
	"context"
	"fmt"
	"os"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ingestResultsIn struct {
	PK string `json:"pk" jsonschema:"data.go.kr publicDataPk of the 개표결과 file dataset to download and ingest"`
}
type ingestSummary struct {
	DatasetID int64  `json:"datasetId"`
	Rows      int    `json:"rows"`
	Message   string `json:"message"`
}
type ingestPollsIn struct {
	Board string `json:"board,omitempty" jsonschema:"NESDC board name carrying the cumulative master xlsx (default 'data')"`
}

func registerIngestTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ingest_results",
		Description: "data.go.kr publicDataPk 의 개표결과(CSV)를 키 없이 내려받아 정규화 후 로컬 DB에 적재한다(멱등). XLSX 전용 데이터셋은 미지원.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ingestResultsIn) (*mcp.CallToolResult, *ingestSummary, error) {
		dir, err := os.MkdirTemp("", "kvote-mcp-")
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		defer os.RemoveAll(dir)
		path, err := deps.NEC.Download(ctx, in.PK, dir)
		if err != nil {
			return errResult(fmt.Sprintf("다운로드 실패: %v", err)), nil, nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		recs, err := nec.ParseResults(raw)
		if err != nil {
			return errResult(fmt.Sprintf("정규화 실패(XLSX 전용일 수 있음 — nec pull 로 원본 확인): %v", err)), nil, nil
		}
		db, err := store.Open(deps.DBPath)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		defer db.Close()
		dsID, err := db.IngestResults(store.DatasetMeta{Source: "nec", PublicDataPk: in.PK, Name: path}, recs)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &ingestSummary{DatasetID: dsID, Rows: len(recs),
			Message: fmt.Sprintf("%d개 투표구 적재", len(recs))}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ingest_polls",
		Description: "NESDC 누적 마스터 엑셀(전국 여론조사)을 내려받아 정규화 후 로컬 DB에 적재한다(멱등).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ingestPollsIn) (*mcp.CallToolResult, *ingestSummary, error) {
		boardName := in.Board
		if boardName == "" {
			boardName = "data"
		}
		board, err := nesdc.BoardByName(boardName)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		att, err := deps.NESDC.LatestBulkXlsx(ctx, board)
		if err != nil {
			return errResult(fmt.Sprintf("최신 엑셀 조회 실패: %v", err)), nil, nil
		}
		dir, err := os.MkdirTemp("", "kvote-mcp-poll-")
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		defer os.RemoveAll(dir)
		path, err := deps.NESDC.Download(ctx, att, dir)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		recs, err := nesdc.ParseBulkXlsx(path)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		db, err := store.Open(deps.DBPath)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		defer db.Close()
		dsID, err := db.IngestPolls(store.DatasetMeta{Source: "nesdc", PublicDataPk: "bulk-" + boardName, Name: path}, recs)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &ingestSummary{DatasetID: dsID, Rows: len(recs),
			Message: fmt.Sprintf("%d건 적재", len(recs))}, nil
	})
}
