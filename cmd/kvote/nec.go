package main

import (
	"context"
	"fmt"
	"os"

	"github.com/JungHoonGhae/kvote/internal/nec"
	"github.com/JungHoonGhae/kvote/internal/output"
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
	var file string
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

			recs, err := nec.ParseResults(raw)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "정규화 완료: %d개 투표구\n", len(recs))
			return renderResults(cmd, format, recs)
		},
	}
	c.Flags().StringVar(&file, "file", "", "이미 받은 개표결과 CSV 경로 (다운로드 생략)")
	return c
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
		headers := []string{"시도", "선거구", "읍면동", "투표구", "선거인수", "투표수", "무효", "기권", "후보수"}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			rows = append(rows, []string{
				r.Sido, r.District, r.Town, r.Booth,
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
