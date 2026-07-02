package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
	"github.com/JungHoonGhae/k-vote-cli/internal/output"
	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/spf13/cobra"
)

func dbCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "db",
		Short: "로컬 SQLite 데이터셋 적재·질의 (MCP query 와 같은 store)",
	}
	c.AddCommand(dbIngestCmd(), dbQueryCmd())
	return c
}

func dbIngestCmd() *cobra.Command {
	ingest := &cobra.Command{Use: "ingest", Short: "데이터를 로컬 DB에 적재"}

	results := &cobra.Command{
		Use:   "results <publicDataPk>",
		Short: "개표결과 CSV를 내려받아 적재 (멱등)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, err := resolveDBPath()
			if err != nil {
				return err
			}
			dir, err := os.MkdirTemp("", "kvote-db-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			path, err := newNECClient().Download(context.Background(), args[0], dir)
			if err != nil {
				return err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			recs, err := nec.ParseResults(raw)
			if err != nil {
				return err
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			id, err := db.IngestResults(store.DatasetMeta{Source: "nec", PublicDataPk: args[0], Name: filepath.Base(path)}, recs)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "적재 완료: dataset=%d, %d개 투표구\n", id, len(recs))
			return nil
		},
	}

	polls := &cobra.Command{
		Use:   "polls",
		Short: "NESDC 누적 여론조사 엑셀을 내려받아 적재 (멱등)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dbPath, err := resolveDBPath()
			if err != nil {
				return err
			}
			board, err := nesdc.BoardByName("data")
			if err != nil {
				return err
			}
			client := newNESDCClient()
			ctx := context.Background()
			att, err := client.LatestBulkXlsx(ctx, board)
			if err != nil {
				return err
			}
			dir, err := os.MkdirTemp("", "kvote-db-poll-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			path, err := client.Download(ctx, att, dir)
			if err != nil {
				return err
			}
			recs, err := nesdc.ParseBulkXlsx(path)
			if err != nil {
				return err
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			id, err := db.IngestPolls(store.DatasetMeta{Source: "nesdc", PublicDataPk: "bulk-data", Name: filepath.Base(path)}, recs)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "적재 완료: dataset=%d, %d건\n", id, len(recs))
			return nil
		},
	}

	ingest.AddCommand(results, polls)
	return ingest
}

func dbQueryCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "query <sql>",
		Short: "로컬 DB에 read-only SQL 질의",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			dbPath, err := resolveDBPath()
			if err != nil {
				return err
			}
			db, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			qr, err := db.Query(args[0], limit)
			if err != nil {
				return err
			}
			if format == output.Table {
				rows := make([][]string, len(qr.Rows))
				for i, r := range qr.Rows {
					cells := make([]string, len(r))
					for j, v := range r {
						cells[j] = fmt.Sprintf("%v", v)
					}
					rows[i] = cells
				}
				return output.WriteTable(cmd.OutOrStdout(), qr.Columns, rows)
			}
			return output.WriteJSON(cmd.OutOrStdout(), qr)
		},
	}
	c.Flags().IntVar(&limit, "limit", 1000, "최대 반환 행수")
	return c
}
