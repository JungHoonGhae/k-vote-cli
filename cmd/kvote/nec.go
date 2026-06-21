package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// necCmd is the NEC (중앙선거관리위원회) provider. Implementation is pending: the
// election statistics portal (info.nec.go.kr) disallows crawling via robots.txt
// and the official API requires an issued key. Per the project's keyless-access
// principle, NEC support will source NEC's published open-data files (downloadable
// without a key) rather than scrape the robots-disallowed frameset.
func necCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "nec",
		Short: "중앙선거관리위원회 (nec.go.kr) 선거 데이터 [준비 중]",
		Long: `nec — 중앙선거관리위원회 선거 데이터 provider.

상태: 준비 중.

설계 원칙(키 없는 접근):
  • info.nec.go.kr (선거통계시스템) 은 robots.txt 가 전면 크롤링을 금지합니다.
  • 공식 API 는 키 발급이 필요해 접근성이 낮습니다.
  → 따라서 NEC 는 키 없이 받을 수 있는 공개 데이터 파일
    (data.nec.go.kr / data.go.kr 파일셋, 개표결과·투표율 등) 을 우선 수집합니다.

진행 상황은 README 의 로드맵을 참고하세요.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	c.AddCommand(&cobra.Command{
		Use:   "roadmap",
		Short: "NEC provider 로드맵 출력",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), necRoadmap)
			return nil
		},
	})
	return c
}

const necRoadmap = `NEC provider 로드맵 (키 없는 공개 데이터 우선)

후보 소스:
  1. 개표결과 / 투표율   data.nec.go.kr 또는 data.go.kr 파일셋 (CSV/XLS, 키 불필요)
  2. 후보자 / 당선인     동일
  3. 선거인 명부 통계     동일
보류:
  - info.nec.go.kr 선거통계시스템 직접 스크래핑 (robots.txt 전면 차단 → 지양)
  - 키 발급형 공식 OpenAPI (접근성 원칙상 후순위)`
