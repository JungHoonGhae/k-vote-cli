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
	c.AddCommand(necDatasetsCmd(), necPullCmd(), necResultsCmd(), necLatestCmd(), necTurnoutCmd())
	return c
}

// resolveAPIKey returns the data.go.kr serviceKey from the --api-key flag or,
// if empty, the KVOTE_DATAGOKR_KEY environment variable. The key is secret, so
// the env var is the preferred path (it never lands in shell history or argv).
func resolveAPIKey(flagVal string) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	if k := os.Getenv("KVOTE_DATAGOKR_KEY"); k != "" {
		return k, nil
	}
	return "", fmt.Errorf("data.go.kr 인증키가 필요합니다: 환경변수 KVOTE_DATAGOKR_KEY 설정 또는 --api-key 전달\n" +
		"  키 발급(자동승인): https://www.data.go.kr/data/15000900/openapi.do 활용신청")
}

// necTurnoutCmd queries the data.go.kr OpenAPI for region-level turnout. Unlike
// the file-dataset commands this needs a serviceKey (auto-approved 활용신청), but
// returns structured 투표율 directly — the most robust, official path.
func necTurnoutCmd() *cobra.Command {
	var sgType, apiKey string
	c := &cobra.Command{
		Use:   "turnout <sgId>",
		Short: "투표율(시도/구시군별) — data.go.kr OpenAPI (인증키 필요)",
		Long: `선거ID(sgId, 선거일 YYYYMMDD)와 선거종류코드(--sgtype)로 투표율을
data.go.kr OpenAPI(VoteXmntckInfoInqireService2)에서 받아옵니다. 파일 데이터와
달리 인증키가 필요하지만(자동승인 활용신청), 시도/구시군별 투표율을 구조화된
형태로 직접 줍니다 — 가장 견고한 공식 경로.

선거종류코드(--sgtype): 1=대통령 2=국회의원 3=시도지사 4=구시군장
                         5=시도의원 6=구시군의원 (선거별로 제공 종류가 다름)

인증키: 환경변수 KVOTE_DATAGOKR_KEY 또는 --api-key. 키는 로그에 남기지 않습니다.

예) kvote nec turnout 20250603 --sgtype 1     # 제21대 대선 투표율
    kvote nec turnout 20220601 --sgtype 3     # 제8회 지방 시도지사 투표율`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			key, err := resolveAPIKey(apiKey)
			if err != nil {
				return err
			}
			recs, err := newNECClient().Turnout(context.Background(), key, args[0], sgType)
			if err != nil {
				return err
			}
			if len(recs) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "sgId=%s sgtype=%s 에 대한 투표율 데이터가 없습니다 "+
					"(아직 미발행이거나 선거종류코드를 확인하세요).\n", args[0], sgType)
				return nil
			}
			return renderTurnout(cmd, format, recs)
		},
	}
	c.Flags().StringVar(&sgType, "sgtype", "1", "선거종류코드: 1=대통령 2=국회의원 3=시도지사 4=구시군장 5=시도의원 6=구시군의원")
	c.Flags().StringVar(&apiKey, "api-key", "", "data.go.kr 인증키 (기본: 환경변수 KVOTE_DATAGOKR_KEY)")
	return c
}

func necLatestCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "latest <선거종류 키워드>",
		Short: "선거종류의 최신 회차 데이터셋을 두 소스에서 자동 해석",
		Long: `키워드(예: "지방선거 개표결과", "대통령선거 개표결과")로 data.go.kr 과
개방포털 양쪽에서 제목의 회차(제N회/제N대)를 파싱해 가장 최신 회차를 찾습니다.
새 선거 결과가 올라오는 순간 자동으로 잡히므로, 회차를 외울 필요가 없습니다.

나온 key 를 그대로 ` + "`nec results <key> --source <소스>`" + ` 에 넘기면 정규화됩니다.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			keyword := strings.Join(args, " ")
			refs := newNECClient().LatestDataset(context.Background(), keyword)
			if len(refs) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "%q 에 해당하는 회차 데이터셋을 찾지 못했습니다.\n", keyword)
				return nil
			}
			switch format {
			case output.JSON:
				return output.WriteJSON(cmd.OutOrStdout(), refs)
			case output.JSONL:
				items := make([]any, len(refs))
				for i := range refs {
					items[i] = refs[i]
				}
				return output.WriteJSONL(cmd.OutOrStdout(), items)
			default:
				rows := make([][]string, 0, len(refs))
				for _, r := range refs {
					rows = append(rows, []string{r.Source, r.Key, fmt.Sprintf("제%d", r.Era), r.Title})
				}
				return output.WriteTable(cmd.OutOrStdout(), []string{"source", "key", "회차", "title"}, rows)
			}
		},
	}
	return c
}

func necDatasetsCmd() *cobra.Command {
	var query, org, source string
	var page int
	c := &cobra.Command{
		Use:   "datasets",
		Short: "선관위 공개 파일 데이터 검색 (개표결과·투표율 등)",
		Long: `선관위가 공개한 파일 데이터를 검색합니다. 두 소스 지원:
  --source datagokr  (기본) data.go.kr — 항목 키는 publicDataPk
  --source openportal      data.nec.go.kr 개방포털 — 항목 키는 dataId,
                           투표율·당선인·후보자 등 더 많은 종류 + XLSX 직접 다운로드
검색어 없이 실행하면 나열합니다. 항목 키를 ` + "`nec pull`" + ` 에 넘겨 다운로드합니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			if source == string(nec.SourceOpenPortal) {
				ds, err := newNECClient().OpenPortalDatasets(context.Background(), query)
				if err != nil {
					return err
				}
				return renderOPDatasets(cmd, format, ds)
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
	f.StringVar(&org, "org", nec.DefaultOrg, "발행 기관명 (datagokr 전용)")
	f.IntVar(&page, "page", 1, "페이지 번호 (datagokr 전용)")
	f.StringVar(&source, "source", string(nec.SourceDataGoKr), "소스: datagokr | openportal")
	return c
}

func necPullCmd() *cobra.Command {
	var outDir, source string
	c := &cobra.Command{
		Use:   "pull <publicDataPk|dataId>",
		Short: "파일 데이터 다운로드 (CSV/XLSX 원본)",
		Long: `파일 데이터를 다운로드합니다.
  --source datagokr  (기본) data.go.kr — 인자는 publicDataPk
  --source openportal      data.nec.go.kr 개방포털 — 인자는 dataId.
                           직접 호스팅 파일(예: 대선 XLSX)을 받습니다. 일부 데이터셋(예:
                           지방선거)은 개방포털이 data.go.kr 로 라우팅하므로 그 경우는
                           datagokr 소스를 쓰세요.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newNECClient()
			ctx := context.Background()
			dir := outDir
			if dir == "" {
				dir = "downloads/nec"
			}
			if source == string(nec.SourceOpenPortal) {
				files, err := client.OpenPortalFiles(ctx, args[0])
				if err != nil {
					return err
				}
				if len(files) == 0 {
					return fmt.Errorf("개방포털 dataId=%s 에 직접 다운로드 파일이 없습니다 (data.go.kr 로 라우팅되는 데이터셋일 수 있음 — --source datagokr 시도)", args[0])
				}
				var failed int
				for _, f := range files {
					path, err := client.OpenPortalDownload(ctx, f.AttachFileID, dir)
					if err != nil {
						failed++
						fmt.Fprintf(cmd.ErrOrStderr(), "  ✗ %s: %v\n", f.Name, err)
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %s\n", path)
				}
				if failed > 0 {
					return fmt.Errorf("%d/%d 파일 실패", failed, len(files))
				}
				return nil
			}
			path, err := client.Download(ctx, args[0], dir)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  ✓ %s\n", path)
			return nil
		},
	}
	c.Flags().StringVarP(&outDir, "out", "o", "", "저장 디렉터리 (기본: downloads/nec)")
	c.Flags().StringVar(&source, "source", string(nec.SourceDataGoKr), "소스: datagokr | openportal")
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

func renderTurnout(cmd *cobra.Command, format output.Format, recs []nec.TurnoutRecord) error {
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
		headers := []string{"시도", "구시군", "선거인수", "투표수", "투표율(%)"}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			rows = append(rows, []string{
				r.Sido, r.Gusigun, fmt.Sprint(r.Electorate), fmt.Sprint(r.Votes),
				fmt.Sprintf("%.1f", r.Turnout),
			})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}

func renderOPDatasets(cmd *cobra.Command, format output.Format, ds []nec.OPDataset) error {
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
		rows := make([][]string, 0, len(ds))
		for _, d := range ds {
			rows = append(rows, []string{d.DataID, d.Title})
		}
		return output.WriteTable(cmd.OutOrStdout(), []string{"dataId", "title"}, rows)
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
