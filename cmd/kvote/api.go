package main

import (
	"errors"
	"fmt"

	"github.com/JungHoonGhae/kvote-cli/internal/datagokr"
	"github.com/JungHoonGhae/kvote-cli/internal/output"
	"github.com/spf13/cobra"
)

// apiCmd manages a logged-in data.go.kr account so OpenAPI access (활용신청) can
// be driven from the CLI. Login is interactive once (a browser window); kvote
// keeps that browser alive and re-attaches to it over the Chrome DevTools
// Protocol for later commands — no keychain prompt, no re-login, no session
// replay.
func apiCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "api",
		Short: "data.go.kr 계정 연동 (OpenAPI 활용신청·신청목록·만료 관리)",
		Long: `api — data.go.kr 로그인 세션으로 OpenAPI 활용신청을 관리합니다.

data.go.kr 의 OpenAPI 활용신청은 CAPTCHA + 소셜 로그인 뒤에 있어 완전 자동화가
안 됩니다. 대신 ` + "`kvote api login`" + ` 으로 브라우저에서 한 번 로그인하면,
kvote 가 그 브라우저를 살려두고 Chrome DevTools Protocol 로 다시 붙어
이후 명령들을 처리합니다 — 키체인 묻지 않음, 재로그인 없음.`,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(apiLoginCmd(), apiListCmd(), apiLogoutCmd())
	return c
}

func apiLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "브라우저로 data.go.kr 에 로그인 (세션을 살려둠)",
		Long: `브라우저 창을 띄워 data.go.kr 에 로그인합니다. 로그인이 끝나면 kvote 가
그 브라우저를 백그라운드로 유지하고, 이후 ` + "`kvote api list`" + ` 등은
그 세션에 다시 붙어 동작합니다. 키체인 비밀번호는 묻지 않습니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := datagokr.Login(cmd.Context(), cmd.ErrOrStderr()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "   이제 `kvote api list` 로 활용신청 현황을 볼 수 있습니다.")
			return nil
		},
	}
}

func apiListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "내 OpenAPI 활용신청 현황 (상태·만료예정일)",
		Long: `살아있는 브라우저 세션에 붙어 data.go.kr 활용신청 현황을 조회합니다.
각 신청건의 상태·신청일·만료예정일을 보여줍니다. 세션이 없으면
` + "`kvote api login`" + ` 을 먼저 실행하라고 안내합니다.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			apps, err := datagokr.Applications(cmd.Context())
			if err != nil {
				if errors.Is(err, datagokr.ErrNotLoggedIn) {
					fmt.Fprintln(cmd.ErrOrStderr(), err)
					return nil
				}
				return err
			}
			if len(apps) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "활용신청 내역이 없습니다.")
				return nil
			}
			return renderApplications(cmd, format, apps)
		},
	}
}

func apiLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "세션 브라우저를 닫고 상태를 정리",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := datagokr.Logout(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "세션을 종료했습니다.")
			return nil
		},
	}
}

func renderApplications(cmd *cobra.Command, format output.Format, apps []datagokr.Application) error {
	switch format {
	case output.JSON:
		return output.WriteJSON(cmd.OutOrStdout(), apps)
	case output.JSONL:
		items := make([]any, len(apps))
		for i := range apps {
			items[i] = apps[i]
		}
		return output.WriteJSONL(cmd.OutOrStdout(), items)
	default:
		headers := []string{"상태", "계정", "데이터명", "제공기관", "신청일", "만료예정일"}
		rows := make([][]string, 0, len(apps))
		for _, a := range apps {
			rows = append(rows, []string{a.Status, a.Account, a.Title, a.Org, a.AppliedAt, a.ExpiresAt})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}
