package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
	"github.com/JungHoonGhae/k-vote-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func nesdcCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "nesdc",
		Short: "중앙선거여론조사심의위원회 (nesdc.go.kr) 여론조사 데이터",
	}
	c.AddCommand(nesdcShowCmd(), nesdcPullCmd(), nesdcSyncCmd(), nesdcBulkCmd())
	return c
}

// ---- boards ----

// ---- shared filter flags (sync) ----

// filterFlags are the search/filter flags shared by `list` and `sync`. They map
// friendly names to the board's raw query codes.
type filterFlags struct {
	query, field, dateField, from, to, gubun string
}

func (ff *filterFlags) register(f *pflag.FlagSet) {
	f.StringVarP(&ff.query, "query", "q", "", "검색어 (searchWrd)")
	f.StringVar(&ff.field, "field", "", "검색 필드: "+strings.Join(nesdc.SortedKeys(nesdc.SearchField), "|")+" (또는 원시 코드)")
	f.StringVar(&ff.dateField, "date-field", "registered", "기간 기준: "+strings.Join(nesdc.SortedKeys(nesdc.DateField), "|"))
	f.StringVar(&ff.from, "from", "", "시작일 (YYYY-MM-DD)")
	f.StringVar(&ff.to, "to", "", "종료일 (YYYY-MM-DD)")
	f.StringVar(&ff.gubun, "gubun", "", "선거구분 코드 (results 전용)")
}

func (ff *filterFlags) options(page int) nesdc.ListOptions {
	return nesdc.ListOptions{
		Page:       page,
		Keyword:    ff.query,
		SearchCnd:  nesdc.ResolveSearchField(ff.field),
		SearchTime: nesdc.ResolveDateField(ff.dateField),
		From:       ff.from,
		To:         ff.to,
		PollGubun:  ff.gubun,
	}
}

// ---- show ----

func nesdcShowCmd() *cobra.Command {
	var boardName string
	var crosstab bool
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
			if crosstab {
				sc := nesdc.SampleCompositionOf(d)
				if sc == nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "표본 구성 교차표를 찾지 못했습니다.")
					return nil
				}
				return renderComposition(cmd, format, sc)
			}
			return renderDetail(cmd, format, d)
		},
	}
	c.Flags().StringVar(&boardName, "board", "results", "게시판 이름")
	c.Flags().BoolVar(&crosstab, "crosstab", false, "표본 구성 교차표(성별·연령·지역 × 완료·가중)만 출력")
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
	var ff filterFlags
	var outDir string
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
				res, err := client.List(ctx, board, ff.options(page))
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
	ff.register(f)
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

// ---- bulk ----

func nesdcBulkCmd() *cobra.Command {
	var boardName, saveDir string
	c := &cobra.Command{
		Use:   "bulk",
		Short: "주차별 누적 마스터 엑셀을 받아 정규화된 여론조사 레코드로 출력",
		Long: `data 게시판 최신 글에 첨부된 누적 마스터 엑셀(전국단위 선거여론조사결과의
주요 데이터)을 내려받아 시트(기간)별로 한 건씩 정규화한 뒤 --format 으로 출력합니다.
2023.10.30 이후 전체 정당지지도 조사가 한 파일에 누적돼 있어 대규모 분석에 적합합니다.

--save <dir> 를 주면 파싱과 별개로 원본 엑셀 파일도 보존합니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			board, err := nesdc.BoardByName(boardName)
			if err != nil {
				return err
			}
			client := newNESDCClient()
			ctx := context.Background()

			att, err := client.LatestBulkXlsx(ctx, board)
			if err != nil {
				return err
			}

			dir := saveDir
			cleanup := false
			if dir == "" {
				dir, err = os.MkdirTemp("", "kvote-bulk-")
				if err != nil {
					return err
				}
				cleanup = true
				defer os.RemoveAll(dir)
			}
			path, err := client.Download(ctx, att, dir)
			if err != nil {
				return err
			}
			if !cleanup {
				fmt.Fprintf(cmd.ErrOrStderr(), "원본 저장: %s\n", path)
			}

			records, err := nesdc.ParseBulkXlsx(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "파싱 완료: %d건\n", len(records))
			return renderBulk(cmd, format, records)
		},
	}
	f := c.Flags()
	f.StringVar(&boardName, "board", "data", "게시판 이름 (누적 엑셀이 붙는 게시판)")
	f.StringVar(&saveDir, "save", "", "원본 엑셀을 보존할 디렉터리 (기본: 임시 후 삭제)")
	return c
}

func renderBulk(cmd *cobra.Command, format output.Format, recs []nesdc.PollRecord) error {
	switch format {
	case output.JSON:
		return output.WriteJSON(cmd.OutOrStdout(), recs)
	case output.JSONL:
		items := make([]any, len(recs))
		for i := range recs {
			items[i] = recs[i]
		}
		return output.WriteJSONL(cmd.OutOrStdout(), items)
	default:
		headers := []string{"기간", "등록번호", "조사기관", "의뢰자", "조사일자", "방법", "표본수", "응답률", "표본오차", "정당지지율"}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			rows = append(rows, []string{
				r.Period, r.RegNo, r.Agency, r.Client, r.SurveyDate,
				r.Method, r.SampleSize, r.ResponseRate, r.MarginError, partySummary(r.PartySupport),
			})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}

// partySummary renders a party→support map as a compact "당명 값" list for the
// table view. Map order is non-deterministic, so JSON/JSONL is preferred for
// analysis; this is a human-readable digest only.
func partySummary(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+" "+v)
	}
	return strings.Join(parts, ", ")
}

// ---- agencies ----

// ---- helpers ----

func boardArg(args []string) (nesdc.Board, error) {
	name := "results"
	if len(args) == 1 {
		name = args[0]
	}
	return nesdc.BoardByName(name)
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

func renderComposition(cmd *cobra.Command, format output.Format, sc *nesdc.SampleComposition) error {
	if format != output.Table {
		return output.WriteJSON(cmd.OutOrStdout(), sc)
	}
	w := cmd.OutOrStdout()
	rows := make([][]string, 0)
	if sc.Total != nil {
		rows = append(rows, []string{"전체", "", fmt.Sprint(sc.Total.Completed), fmt.Sprint(sc.Total.Weighted)})
	}
	for _, ct := range sc.Crosstabs {
		for _, c := range ct.Cells {
			rows = append(rows, []string{ct.Dimension, c.Category, fmt.Sprint(c.Completed), fmt.Sprint(c.Weighted)})
		}
	}
	if err := output.WriteTable(w, []string{"차원", "범주", "완료", "가중"}, rows); err != nil {
		return err
	}
	if sc.Weighting != "" {
		fmt.Fprintf(w, "\n가중: %s\n", sc.Weighting)
	}
	if sc.MarginError != "" {
		fmt.Fprintf(w, "표본오차: %s\n", sc.MarginError)
	}
	return nil
}
