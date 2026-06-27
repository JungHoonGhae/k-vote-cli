# Security Policy

[한국어](#보안-정책) · English

## Supported Versions

Only the latest released version receives fixes. Please reproduce any issue on
the most recent release (or `main`) before reporting.

| Version | Supported |
| ------- | --------- |
| latest release / `main` | ✅ |
| older releases | ❌ |

## Reporting a Vulnerability

**Do not open a public issue for security problems.**

Preferred: GitHub **private vulnerability reporting** —
repo **Security** tab → **Report a vulnerability**
(<https://github.com/JungHoonGhae/k-vote-cli/security/advisories/new>).

Alternative: email **lucas.ghae@gmail.com** with `[k-vote-cli security]` in the
subject.

Please include: affected version/commit, environment (OS, install method),
reproduction steps, and impact. We aim to acknowledge within a few days. This is
a volunteer-maintained project with no bug bounty.

## Scope

In scope — issues in **this CLI's own code**, e.g.:

- Leaking an OpenAPI key (`KVOTE_DATAGOKR_KEY` / `serviceKey`) into logs, output, or diagnostics
- Local browser-session/profile handling for `api login` (cookie storage, file permissions)
- Command/argument injection, unsafe file writes, path traversal during download
- CI/release supply-chain issues (workflows, build, Homebrew cask)

Out of scope:

- Vulnerabilities in **NEC, NESDC, or data.go.kr servers/APIs** — report those to
  the respective operator, not here. This is an **unofficial** project that reads
  public election data; it is not affiliated with or endorsed by any election body.
- Risks inherent to the project's documented design (keyless access to public data,
  `--delay`-throttled polite scraping).

## When reporting

- **Never include real API keys or serviceKeys.** Redact them or use dummy values —
  the same rule the project follows everywhere.

---

## 보안 정책

### 지원 버전

최신 릴리즈만 수정 대상입니다. 신고 전 최신 릴리즈(또는 `main`)에서 재현해 주세요.

### 취약점 신고

**보안 문제는 공개 이슈로 올리지 마세요.**

- 권장: 레포 **Security** 탭 → **Report a vulnerability**
  (<https://github.com/JungHoonGhae/k-vote-cli/security/advisories/new>)
- 또는: **lucas.ghae@gmail.com** 으로 메일 (제목에 `[k-vote-cli security]`)

영향 버전/커밋, 환경(OS·설치 방법), 재현 절차, 영향 범위를 포함해 주세요. 며칠 내
확인을 목표로 하며, 자원봉사로 운영되는 프로젝트라 버그 바운티는 없습니다.

### 범위

대상 — **이 CLI 자체 코드**의 문제: OpenAPI 키(`KVOTE_DATAGOKR_KEY`) 유출, `api login`
브라우저 세션/프로필 처리, 다운로드 중 명령·인자 인젝션이나 경로 조작, CI/릴리즈 공급망 등.

대상 아님 — **NEC·NESDC·data.go.kr 서버/API** 자체의 취약점(해당 기관에 신고). 본
프로젝트는 공개 선거 데이터를 읽는 **비공식** 도구로, 어떤 선거 기관과도 무관합니다.

### 신고 시

- **실제 API 키·serviceKey를 절대 포함하지 마세요.** redact 하거나 더미 값을 쓰세요 —
  프로젝트가 모든 곳에서 지키는 규칙입니다.
