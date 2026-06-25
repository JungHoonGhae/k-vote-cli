package main

import (
	"fmt"
	"os"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
	"github.com/JungHoonGhae/k-vote-cli/internal/output"
	"github.com/spf13/cobra"
)

// doctorCmd smoke-tests the killer data paths against their live sources. Its
// job is to catch breakage caused by upstream site/format changes — the risk
// kvote accepts in exchange for keyless access. Unit tests cover parsing logic;
// doctor covers "does the real source still respond in the shape we expect".
func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "킬러 경로 라이브 점검 (사이트 개편 등 깨짐 감지)",
		Long: `핵심 데이터 경로가 실제 출처에 대해 동작하는지 점검합니다.

  · NEC 파일데이터(data.go.kr) 검색 + corpus manifest pk 유효성
  · NESDC 목록 스크래핑(가장 취약 — 마크업 의존)
  · NESDC 누적 엑셀(bulk) 위치
  · OpenAPI 선거코드 (KVOTE_DATAGOKR_KEY 있을 때만)

하나라도 실패하면 비정상 종료 코드를 반환합니다(CI 용). 파싱 로직은 단위
테스트가, 라이브 출처 변화는 이 명령이 잡습니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			type result struct {
				name, detail string
				ok, skipped  bool
			}
			var results []result
			add := func(name string, fn func() (string, error)) {
				detail, err := fn()
				if err != nil {
					results = append(results, result{name: name, detail: err.Error()})
					return
				}
				results = append(results, result{name: name, detail: detail, ok: true})
			}

			add("NEC 파일데이터 (data.go.kr)", func() (string, error) {
				ds, err := newNECClient().Datasets(ctx, nec.SearchOptions{Keyword: "개표결과"})
				if err != nil {
					return "", err
				}
				if len(ds) == 0 {
					return "", fmt.Errorf("검색 결과 0 — 포털 변경 의심")
				}
				want := nec.CoreCorpus[0].PublicDataPk
				for _, d := range ds {
					if d.PublicDataPk == want {
						return fmt.Sprintf("%d개 데이터셋, corpus pk %s 유효", len(ds), want), nil
					}
				}
				return fmt.Sprintf("%d개 (corpus pk %s 미발견 — manifest 점검)", len(ds), want), nil
			})

			add("NESDC 목록 스크래핑 (results)", func() (string, error) {
				b, err := nesdc.BoardByName("results")
				if err != nil {
					return "", err
				}
				res, err := newNESDCClient().List(ctx, b, nesdc.ListOptions{})
				if err != nil {
					return "", err
				}
				if len(res.Items) == 0 {
					return "", fmt.Errorf("행 0 — 마크업 변경 의심")
				}
				return fmt.Sprintf("%d행 · %d열", len(res.Items), len(res.Columns)), nil
			})

			add("NESDC 누적 엑셀 위치 (data/bulk)", func() (string, error) {
				b, err := nesdc.BoardByName("data")
				if err != nil {
					return "", err
				}
				att, err := newNESDCClient().LatestBulkXlsx(ctx, b)
				if err != nil {
					return "", err
				}
				if att.Name == "" {
					return "", fmt.Errorf("xlsx 첨부 미발견")
				}
				return att.Name, nil
			})

			if key := os.Getenv("KVOTE_DATAGOKR_KEY"); key != "" {
				add("OpenAPI 선거코드 (data.go.kr)", func() (string, error) {
					els, err := newNECClient().Elections(ctx, key)
					if err != nil {
						return "", err
					}
					if len(els) == 0 {
						return "", fmt.Errorf("선거코드 0")
					}
					return fmt.Sprintf("%d개 선거코드", len(els)), nil
				})
			} else {
				results = append(results, result{
					name: "OpenAPI (선택)", skipped: true,
					detail: "KVOTE_DATAGOKR_KEY 없음 — 건너뜀",
				})
			}

			// 렌더 + 요약.
			rows := make([][]string, 0, len(results))
			failed := 0
			for _, r := range results {
				status := "✅ OK"
				if r.skipped {
					status = "— SKIP"
				} else if !r.ok {
					status = "❌ FAIL"
					failed++
				}
				rows = append(rows, []string{status, r.name, r.detail})
			}
			if err := output.WriteTable(cmd.OutOrStdout(), []string{"상태", "점검", "상세"}, rows); err != nil {
				return err
			}
			if failed > 0 {
				return fmt.Errorf("%d개 점검 실패 — 킬러 경로 점검 필요", failed)
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "모든 킬러 경로 정상.")
			return nil
		},
	}
}
