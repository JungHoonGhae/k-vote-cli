package main

import (
	"time"

	"github.com/JungHoonGhae/kvote-cli/internal/nec"
	"github.com/JungHoonGhae/kvote-cli/internal/nesdc"
	"github.com/JungHoonGhae/kvote-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	flagFormat  string
	flagDelay   time.Duration
	flagBaseURL string
)

var rootCmd = &cobra.Command{
	Use:   "kvote",
	Short: "한국 선거 데이터 수집 CLI (NESDC 여론조사 · NEC 선거)",
	Long: `kvote — 한국 선거 관련 공개 데이터를 수집하는 비공식 CLI.

데이터 출처(provider):
  nesdc   중앙선거여론조사심의위원회 (nesdc.go.kr) — 여론조사 결과·기관 현황
  nec     중앙선거관리위원회 (nec.go.kr) — 선거 통계 (키 없는 공개 데이터)

모든 출력은 기본 JSON이며 --format 으로 jsonl/table 선택 가능합니다.
API 키 발급이 필요 없습니다.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVarP(&flagFormat, "format", "f", "json", "출력 형식: json | jsonl | table")
	pf.DurationVar(&flagDelay, "delay", nesdc.DefaultDelay, "요청 간 최소 간격 (rate limit)")
	pf.StringVar(&flagBaseURL, "base-url", "", "포털 base URL 재정의 (테스트용)")

	rootCmd.AddCommand(nesdcCmd())
	rootCmd.AddCommand(necCmd())
	rootCmd.AddCommand(apiCmd())
	rootCmd.AddCommand(versionCmd())
}

// resolveFormat parses the --format flag.
func resolveFormat() (output.Format, error) {
	return output.Parse(flagFormat)
}

// newNESDCClient builds a NESDC client from the global flags.
func newNESDCClient() *nesdc.Client {
	opts := []nesdc.Option{nesdc.WithDelay(flagDelay)}
	if flagBaseURL != "" {
		opts = append(opts, nesdc.WithBaseURL(flagBaseURL))
	}
	return nesdc.New(opts...)
}

// newNECClient builds a NEC (data.go.kr) client from the global flags.
func newNECClient() *nec.Client {
	opts := []nec.Option{nec.WithDelay(flagDelay)}
	if flagBaseURL != "" {
		opts = append(opts, nec.WithBaseURL(flagBaseURL))
	}
	return nec.New(opts...)
}
