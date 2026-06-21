package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/JungHoonGhae/kvote-cli/internal/nec"
	"github.com/JungHoonGhae/kvote-cli/internal/output"
	"github.com/spf13/cobra"
)

// necCmd is the NEC (중앙선거관리위원회) provider. It sources NEC election
// results from the open-data portal data.go.kr, where the commission publishes
// 개표결과·투표율 as file datasets (CSV/XLSX) that download without an API key.
// The robots-disallowed info.nec.go.kr statistics site is deliberately not
// scraped; data.go.kr is the sanctioned, keyless distribution channel.
func necCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "nec",
		Short: "중앙선거관리위원회 선거 데이터 (data.go.kr 공개 파일, 키 불필요)",
		Long: `nec — 중앙선거관리위원회 선거 데이터 provider.

선관위가 data.go.kr 에 공개한 개표결과·투표율 등 파일 데이터(CSV/XLSX)를
API 키 없이 검색·다운로드합니다. (info.nec.go.kr 선거통계시스템은 robots 전면
차단이라 스크래핑하지 않고, 공식 배포 채널인 data.go.kr 를 사용합니다.)`,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(necDatasetsCmd(), necPullCmd(), necResultsCmd())
	return c
}

func necDatasetsCmd() *cobra.Command {
	var query, org string
	var page int
	c := &cobra.Command{
		Use:   "datasets",
		Short: "선관위 공개 파일 데이터 검색 (개표결과·투표율 등)",
		Long: `data.go.kr 에서 선관위가 공개한 파일 데이터를 검색합니다.
검색어 없이 실행하면 선관위 데이터셋을 나열합니다. 각 항목의 publicDataPk 를
` + "`nec pull <publicDataPk>`" + ` 에 넘겨 다운로드합니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			ds, err := newNECClient().Datasets(context.Background(), nec.SearchOptions{
				Keyword: query, Org: org, Page: page,
			})
			if err != nil {
				return err
			}
			return renderDatasets(cmd, format, ds)
		},
	}
	f := c.Flags()
	f.StringVarP(&query, "query", "q", "", "검색어 (예: 개표결과, 투표율, 대통령선거)")
	f.StringVar(&org, "org", nec.DefaultOrg, "발행 기관명")
	f.IntVar(&page, "page", 1, "페이지 번호")
	return c
}

func necPullCmd() *cobra.Command {
	var outDir string
	c := &cobra.Command{
		Use:   "pull <publicDataPk>",
		Short: "파일 데이터 다운로드 (CSV/XLSX 원본)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newNECClient()
			dir := outDir
			if dir == "" {
				dir = "downloads/nec"
			}
			path, err := client.Download(context.Background(), args[0], dir)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %s\n", path)
			return nil
		},
	}
	c.Flags().StringVarP(&outDir, "out", "o", "", "저장 디렉터리 (기본: downloads/nec)")
	return c
}

func necResultsCmd() *cobra.Command {
	var file, aggregate string
	var byVoteType bool
	var race string
	var leafOnly bool
	c := &cobra.Command{
		Use:   "results <publicDataPk>",
		Short: "개표결과 CSV를 투표구별 정규화 레코드로 출력",
		Long: `개표결과 파일 데이터(CSV)를 받아 투표구(시도→선거구→읍면동→투표구)별로
선거인수·투표수·무효투표수·기권자수와 후보자별 득표를 한 레코드로 정규화합니다.
대용량이라 --format jsonl 을 권장합니다.

--file 로 이미 내려받은 CSV 경로를 주면 다운로드 없이 파싱합니다.
(XLSX 로만 배포되는 데이터셋은 nec pull 로 원본을 받으세요.)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			var raw []byte
			switch {
			case file != "":
				if raw, err = os.ReadFile(file); err != nil {
					return err
				}
			case len(args) == 1:
				dir, err := os.MkdirTemp("", "kvote-nec-")
				if err != nil {
					return err
				}
				defer os.RemoveAll(dir)
				path, err := newNECClient().Download(context.Background(), args[0], dir)
				if err != nil {
					return err
				}
				if raw, err = os.ReadFile(path); err != nil {
					return err
				}
			default:
				return fmt.Errorf("publicDataPk 또는 --file 중 하나가 필요합니다")
			}

			if isXLSX(raw) {
				if aggregate != "none" {
					fmt.Fprintln(cmd.ErrOrStderr(), "주의: --aggregate 는 XLSX 에 미지원입니다 (P2 비범위). --leaf-only 로 leaf 행만 거를 수 있습니다.")
				}
				ers, err := nec.ParseResultsXLSX(raw)
				if err != nil {
					return err
				}
				if race != "" {
					ers = filterRace(ers, race)
				}
				if leafOnly {
					ers = filterLeaf(ers)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "정규화 완료: %d개 행 (XLSX)\n", len(ers))
				return renderElection(cmd, format, ers)
			}

			recs, err := nec.ParseResults(raw)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "정규화 완료: %d개 투표구\n", len(recs))

			level, ok := parseAggLevel(aggregate)
			if !ok {
				return fmt.Errorf("알 수 없는 --aggregate 값 %q (none|town|sgg|sido|national)", aggregate)
			}
			if level == nec.AggNone {
				return renderResults(cmd, format, recs)
			}
			aggs := nec.Aggregate(recs, level, byVoteType)
			fmt.Fprintf(cmd.ErrOrStderr(), "집계 완료: %d개 그룹 (level=%s)\n", len(aggs), aggregate)
			return renderAggregated(cmd, format, aggs)
		},
	}
	c.Flags().StringVar(&file, "file", "", "이미 받은 개표결과 CSV 경로 (다운로드 생략)")
	c.Flags().StringVar(&aggregate, "aggregate", "none", "집계 단위: none|town|sgg|sido|national")
	c.Flags().BoolVar(&byVoteType, "by-votetype", false, "투표유형(본/관내사전/관외사전/거소선상)으로 분리 (집계와 함께)")
	c.Flags().StringVar(&race, "race", "", "XLSX 선거종류(시트명) 부분일치 필터")
	c.Flags().BoolVar(&leafOnly, "leaf-only", false, "XLSX 집계행(합계/소계) 제외, leaf만")
	return c
}

// isXLSX reports whether raw is an XLSX (zip) file by its magic bytes.
func isXLSX(raw []byte) bool {
	return len(raw) >= 2 && raw[0] == 'P' && raw[1] == 'K'
}

func filterRace(ers []nec.ElectionResult, race string) []nec.ElectionResult {
	out := ers[:0:0]
	for _, e := range ers {
		if strings.Contains(e.Race, race) {
			out = append(out, e)
		}
	}
	return out
}

func filterLeaf(ers []nec.ElectionResult) []nec.ElectionResult {
	out := ers[:0:0]
	for _, e := range ers {
		if !e.Aggregate {
			out = append(out, e)
		}
	}
	return out
}

func renderElection(cmd *cobra.Command, format output.Format, ers []nec.ElectionResult) error {
	switch format {
	case output.JSON:
		return output.WriteJSON(cmd.OutOrStdout(), ers)
	case output.JSONL:
		items := make([]any, len(ers))
		for i := range ers {
			items[i] = ers[i]
		}
		return output.WriteJSONL(cmd.OutOrStdout(), items)
	default:
		headers := []string{"race", "차원", "투표유형", "집계", "선거인수", "투표수", "무효", "후보수"}
		rows := make([][]string, 0, len(ers))
		for _, e := range ers {
			dims := make([]string, 0, len(e.Dimensions))
			for _, d := range e.Dimensions {
				if d.Value != "" {
					dims = append(dims, d.Value)
				}
			}
			rows = append(rows, []string{
				e.Race, strings.Join(dims, ">"), e.VoteType, fmt.Sprint(e.Aggregate),
				fmt.Sprint(e.Electorate), fmt.Sprint(e.Votes), fmt.Sprint(e.Invalid), fmt.Sprint(len(e.Candidates)),
			})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}

func parseAggLevel(s string) (nec.AggLevel, bool) {
	switch nec.AggLevel(s) {
	case nec.AggNone, nec.AggTown, nec.AggSgg, nec.AggSido, nec.AggNational:
		return nec.AggLevel(s), true
	}
	return "", false
}

func renderAggregated(cmd *cobra.Command, format output.Format, recs []nec.AggregatedRecord) error {
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
		headers := []string{"level", "시도", "선거구", "읍면동", "투표유형", "선거인수", "투표수", "투표율", "후보수"}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			rows = append(rows, []string{
				r.Level, r.Sido, r.District, r.Town, r.VoteType,
				fmt.Sprint(r.Electorate), fmt.Sprint(r.Votes),
				fmt.Sprintf("%.1f%%", r.Turnout*100), fmt.Sprint(len(r.Candidates)),
			})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}

func renderResults(cmd *cobra.Command, format output.Format, recs []nec.ResultRecord) error {
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
		headers := []string{"시도", "선거구", "읍면동", "투표구", "투표유형", "선거인수", "투표수", "무효", "기권", "후보수"}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			rows = append(rows, []string{
				r.Sido, r.District, r.Town, r.Booth, r.VoteType,
				fmt.Sprint(r.Electorate), fmt.Sprint(r.Votes), fmt.Sprint(r.Invalid),
				fmt.Sprint(r.Abstention), fmt.Sprint(len(r.Candidates)),
			})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}

func renderDatasets(cmd *cobra.Command, format output.Format, ds []nec.Dataset) error {
	switch format {
	case output.JSON:
		return output.WriteJSON(cmd.OutOrStdout(), ds)
	case output.JSONL:
		items := make([]any, len(ds))
		for i := range ds {
			items[i] = ds[i]
		}
		return output.WriteJSONL(cmd.OutOrStdout(), items)
	default:
		headers := []string{"publicDataPk", "title", "formats"}
		rows := make([][]string, 0, len(ds))
		for _, d := range ds {
			rows = append(rows, []string{d.PublicDataPk, d.Title, fmt.Sprint(d.Formats)})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}
