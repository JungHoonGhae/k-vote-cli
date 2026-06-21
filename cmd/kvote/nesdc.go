package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/JungHoonGhae/kvote/internal/nesdc"
	"github.com/JungHoonGhae/kvote/internal/output"
	"github.com/spf13/cobra"
)

func nesdcCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "nesdc",
		Short: "중앙선거여론조사심의위원회 (nesdc.go.kr) 여론조사 데이터",
	}
	c.AddCommand(nesdcBoardsCmd(), nesdcListCmd(), nesdcShowCmd(), nesdcPullCmd(), nesdcSyncCmd(), nesdcAgenciesCmd())
	return c
}

// ---- boards ----

func nesdcBoardsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "boards",
		Short: "수집 가능한 게시판 목록",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			boards := nesdc.Boards()
			if format == output.Table {
				rows := make([][]string, 0, len(boards))
				for _, b := range boards {
					rows = append(rows, []string{b.Name, b.Title, b.BbsID, b.MenuNo})
				}
				return output.WriteTable(cmd.OutOrStdout(), []string{"name", "title", "bbsId", "menuNo"}, rows)
			}
			return output.WriteJSON(cmd.OutOrStdout(), boards)
		},
	}
}

// ---- list ----

func nesdcListCmd() *cobra.Command {
	var page int
	var query, from, to, gubun string
	c := &cobra.Command{
		Use:   "list [board]",
		Short: "게시판 목록 조회 (기본: results)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			board, err := boardArg(args)
			if err != nil {
				return err
			}
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			res, err := newNESDCClient().List(context.Background(), board, nesdc.ListOptions{
				Page: page, Keyword: query, From: from, To: to, PollGubun: gubun,
			})
			if err != nil {
				return err
			}
			return renderList(cmd, format, res)
		},
	}
	f := c.Flags()
	f.IntVar(&page, "page", 1, "페이지 번호")
	f.StringVarP(&query, "query", "q", "", "검색어")
	f.StringVar(&from, "from", "", "시작일 (YYYY-MM-DD)")
	f.StringVar(&to, "to", "", "종료일 (YYYY-MM-DD)")
	f.StringVar(&gubun, "gubun", "", "선거구분 코드 (results 전용)")
	return c
}

// ---- show ----

func nesdcShowCmd() *cobra.Command {
	var boardName string
	c := &cobra.Command{
		Use:   "show <nttId>",
		Short: "단건 상세 메타데이터 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			board, err := nesdc.BoardByName(boardName)
			if err != nil {
				return err
			}
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			d, err := newNESDCClient().Detail(context.Background(), board, args[0])
			if err != nil {
				return err
			}
			return renderDetail(cmd, format, d)
		},
	}
	c.Flags().StringVar(&boardName, "board", "results", "게시판 이름")
	return c
}

// ---- pull ----

func nesdcPullCmd() *cobra.Command {
	var boardName, outDir string
	c := &cobra.Command{
		Use:   "pull <nttId>",
		Short: "첨부파일(통계표·설문지 등) 다운로드",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			board, err := nesdc.BoardByName(boardName)
			if err != nil {
				return err
			}
			client := newNESDCClient()
			ctx := context.Background()
			d, err := client.Detail(ctx, board, args[0])
			if err != nil {
				return err
			}
			if len(d.Attachments) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "첨부파일이 없습니다.")
				return nil
			}
			dir := outDir
			if dir == "" {
				dir = "downloads/" + args[0]
			}
			var failed int
			for _, a := range d.Attachments {
				path, err := client.Download(ctx, a, dir)
				if err != nil {
					failed++
					fmt.Fprintf(cmd.ErrOrStderr(), "  ✗ %s: %v\n", a.Name, err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %s\n", path)
			}
			if failed > 0 {
				return fmt.Errorf("%d/%d 파일 다운로드 실패", failed, len(d.Attachments))
			}
			return nil
		},
	}
	f := c.Flags()
	f.StringVar(&boardName, "board", "results", "게시판 이름")
	f.StringVarP(&outDir, "out", "o", "", "저장 디렉터리 (기본: downloads/<nttId>)")
	return c
}

// ---- sync ----

func nesdcSyncCmd() *cobra.Command {
	var query, from, to, gubun, outDir string
	var maxPages int
	var pull bool
	c := &cobra.Command{
		Use:   "sync [board]",
		Short: "기간/조건 전체를 페이지네이션하며 JSONL로 일괄 수집",
		Long: `목록을 끝까지(또는 --max-pages 까지) 순회하며 각 항목을 JSONL 한 줄로 출력합니다.
--pull 을 주면 각 항목의 첨부파일도 --out 디렉터리에 내려받습니다.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			board, err := boardArg(args)
			if err != nil {
				return err
			}
			client := newNESDCClient()
			ctx := context.Background()
			enc := output.NewLineEncoder(cmd.OutOrStdout())

			total := 0
			for page := 1; maxPages == 0 || page <= maxPages; page++ {
				res, err := client.List(ctx, board, nesdc.ListOptions{
					Page: page, Keyword: query, From: from, To: to, PollGubun: gubun,
				})
				if err != nil {
					return err
				}
				if len(res.Items) == 0 {
					break
				}
				for _, item := range res.Items {
					if err := enc.Encode(item); err != nil {
						return err
					}
					total++
					if pull {
						dir := outDir
						if dir == "" {
							dir = "downloads/" + item.NttID
						}
						pullAttachments(ctx, client, board, item.NttID, dir, cmd)
					}
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "page %d: +%d (누적 %d)\n", page, len(res.Items), total)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "완료: 총 %d건\n", total)
			return nil
		},
	}
	f := c.Flags()
	f.StringVarP(&query, "query", "q", "", "검색어")
	f.StringVar(&from, "from", "", "시작일 (YYYY-MM-DD)")
	f.StringVar(&to, "to", "", "종료일 (YYYY-MM-DD)")
	f.StringVar(&gubun, "gubun", "", "선거구분 코드 (results 전용)")
	f.IntVar(&maxPages, "max-pages", 0, "최대 페이지 수 (0=빈 페이지까지 전체)")
	f.BoolVar(&pull, "pull", false, "각 항목의 첨부파일도 다운로드")
	f.StringVarP(&outDir, "out", "o", "", "다운로드 루트 (기본: downloads/<nttId>)")
	return c
}

func pullAttachments(ctx context.Context, client *nesdc.Client, board nesdc.Board, nttID, dir string, cmd *cobra.Command) {
	d, err := client.Detail(ctx, board, nttID)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "  ✗ detail %s: %v\n", nttID, err)
		return
	}
	for _, a := range d.Attachments {
		if _, err := client.Download(ctx, a, dir); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  ✗ %s: %v\n", a.Name, err)
		}
	}
}

// ---- agencies ----

func nesdcAgenciesCmd() *cobra.Command {
	var cancelled bool
	var page int
	var query string
	c := &cobra.Command{
		Use:   "agencies",
		Short: "여론조사기관 등록현황 (--cancelled: 취소현황)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			res, err := newNESDCClient().Agencies(context.Background(), cancelled, nesdc.ListOptions{
				Page: page, Keyword: query,
			})
			if err != nil {
				return err
			}
			return renderAgencies(cmd, format, res)
		},
	}
	f := c.Flags()
	f.BoolVar(&cancelled, "cancelled", false, "취소현황 조회")
	f.IntVar(&page, "page", 1, "페이지 번호")
	f.StringVarP(&query, "query", "q", "", "검색어")
	return c
}

// ---- helpers ----

func boardArg(args []string) (nesdc.Board, error) {
	name := "results"
	if len(args) == 1 {
		name = args[0]
	}
	return nesdc.BoardByName(name)
}

func renderList(cmd *cobra.Command, format output.Format, res *nesdc.ListResult) error {
	switch format {
	case output.JSON:
		return output.WriteJSON(cmd.OutOrStdout(), res)
	case output.JSONL:
		items := make([]any, len(res.Items))
		for i := range res.Items {
			items[i] = res.Items[i]
		}
		return output.WriteJSONL(cmd.OutOrStdout(), items)
	default:
		headers := append([]string{"nttId"}, res.Columns...)
		rows := make([][]string, 0, len(res.Items))
		for _, it := range res.Items {
			row := []string{it.NttID}
			for _, col := range res.Columns {
				row = append(row, it.Values[col])
			}
			rows = append(rows, row)
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}

func renderAgencies(cmd *cobra.Command, format output.Format, res *nesdc.AgencyResult) error {
	switch format {
	case output.JSON:
		return output.WriteJSON(cmd.OutOrStdout(), res)
	case output.JSONL:
		items := make([]any, len(res.Items))
		for i := range res.Items {
			items[i] = res.Items[i]
		}
		return output.WriteJSONL(cmd.OutOrStdout(), items)
	default:
		headers := append([]string{"insttNum"}, res.Columns...)
		rows := make([][]string, 0, len(res.Items))
		for _, it := range res.Items {
			row := []string{it.InsttNum}
			for _, col := range res.Columns {
				row = append(row, it.Values[col])
			}
			rows = append(rows, row)
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}

func renderDetail(cmd *cobra.Command, format output.Format, d *nesdc.Detail) error {
	if format != output.Table {
		return output.WriteJSON(cmd.OutOrStdout(), d)
	}
	w := cmd.OutOrStdout()
	if d.Title != "" {
		fmt.Fprintf(w, "# %s (nttId=%s)\n\n", d.Title, d.NttID)
	}
	rows := make([][]string, 0, len(d.Fields))
	for _, f := range d.Fields {
		rows = append(rows, []string{strings.Join(f.Labels, " / "), strings.Join(f.Values, " | ")})
	}
	if err := output.WriteTable(w, []string{"항목", "값"}, rows); err != nil {
		return err
	}
	if len(d.Attachments) > 0 {
		fmt.Fprintln(w, "\n첨부파일:")
		for _, a := range d.Attachments {
			fmt.Fprintf(w, "  - %s\n", a.Name)
		}
	}
	return nil
}
