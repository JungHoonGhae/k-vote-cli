package main

import (
	"context"

	"github.com/JungHoonGhae/k-vote-cli/internal/mcpserver"
	"github.com/spf13/cobra"
)

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "MCP 서버 실행 (stdio) — AI 에이전트가 탐색·수집·SQL 질의",
		Long: `kvote 를 Model Context Protocol 서버로 노출합니다(stdio).
AI 에이전트가 search_datasets / list_elections / ingest_results / ingest_polls /
query tool 과 kvote://schema 리소스로 한국 선거 공개 데이터를 다룹니다. 키리스·중립.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dbPath, err := resolveDBPath()
			if err != nil {
				return err
			}
			return mcpserver.Serve(context.Background(), mcpserver.Deps{
				DBPath: dbPath,
				NEC:    newNECClient(),
				NESDC:  newNESDCClient(),
			})
		},
	}
}
