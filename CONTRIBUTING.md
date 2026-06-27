# Contributing

k-vote-cli에 기여해 주셔서 감사합니다. 이 프로젝트는 흩어진 한국 선거 공개 데이터의 **접근성**을 높이는 비공식 도구입니다.

## 두 가지 타협 불가 원칙

기여 전에 반드시 이해해 주세요. PR은 이 두 원칙을 기준으로 검토됩니다.

1. **중립성** — kvote는 어떤 분석적 입장도 취하지 않습니다. 데이터 접근성만 높이고, 무엇이 정상·비정상·수상한지 판단하지 않습니다.
   - 제공: 원자료 완전 보존 + *정의가 명시된 재현 가능한 표준 파생값*(비율·합계·집계).
   - 금지: 플래그·점수·순위·"이상치"·"검증 결과"·해석. "수상함"을 내장한 휴리스틱.
   - 판단은 소비자(사람/AI 에이전트)의 몫입니다.
2. **키리스 · 정당한 접근** — 핵심 기능은 API 키 없이 동작해야 합니다. `info.nec.go.kr`처럼 robots로 차단되거나 탐지 기반으로 거부하는 소스를 **우회하지 않습니다.** 대신 선관위의 공식 배포 채널(data.go.kr 파일 데이터, NESDC 게시판)을 씁니다.

## 개발 환경

- Go 1.26+
- Make
- (선택) Google Chrome 계열 — `api login`(data.go.kr OpenAPI 활용신청 자동화)에만 필요. 핵심 기능에는 불필요.

```bash
git clone https://github.com/JungHoonGhae/k-vote-cli.git
cd k-vote-cli
make build   # bin/kvote
make test    # go test ./... — 네트워크 없이 testdata 픽스처로 파서 검증
make fmt     # gofmt
go vet ./...
```

## 브랜치 & 커밋

- `main`에서 feature 브랜치를 생성합니다.
- 커밋 메시지는 [Conventional Commits](https://www.conventionalcommits.org/) 스타일을 따릅니다. 한글 사용 가능.
  - `feat(nec): 사전투표 분리 집계 추가`
  - `fix(nesdc): 첨부 파일명 인코딩 복구`
  - `docs: README 업데이트`

## PR 가이드

1. `make test` 와 `go vet ./...` 가 통과하는지 확인합니다. (CI가 동일하게 검증합니다.)
2. PR 템플릿의 체크리스트를 채워주세요.
3. 새 파생값을 추가한다면 **정의가 코드/문서에 명시**되어야 하고, 해석·판단이 섞이지 않아야 합니다.
4. 마크업/응답 포맷이 바뀌어 파서를 고쳤다면 `testdata` 픽스처도 최신본으로 교체해 주세요.

## 프로젝트 구조

```
cmd/kvote/          CLI (cobra). provider 별 명령 그룹.
internal/nec/       NEC — data.go.kr 파일 데이터(개표결과·투표율) 클라이언트
internal/nesdc/     NESDC — 여론조사 게시판 스크래핑 클라이언트
internal/datagokr/  data.go.kr OpenAPI 활용신청 자동화 (CDP 브라우저 세션)
internal/output/    json / jsonl / table 렌더러 (한글 폭 보정)
```

자세한 설계 포인트는 [`CLAUDE.md`](CLAUDE.md)에 정리돼 있습니다.

## 주의사항

- 이 프로젝트는 NEC·NESDC·data.go.kr의 공개 데이터에 의존합니다. 마크업·응답 포맷은 예고 없이 바뀔 수 있습니다.
- **API 키·serviceKey는 비밀입니다.** 코드·커밋·로그에 절대 넣지 마세요. 환경변수(`KVOTE_DATAGOKR_KEY`)로만 전달합니다.
- `api login`은 사용자가 브라우저에서 직접 로그인하는 방식입니다 — kvote는 비밀번호를 보거나 저장하지 않습니다.
