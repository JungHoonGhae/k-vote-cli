# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 개요

`kvote` 는 한국 선거 관련 공개 데이터를 수집하는 비공식 Go CLI 입니다. **멀티-프로바이더**
구조로, 현재 NESDC(중앙선거여론조사심의위원회) provider 가 구현돼 있고 NEC(중앙선거관리위원회)
는 준비 중입니다. **API 키 발급이 필요 없는** 키리스 접근이 핵심 설계 원칙입니다.

## 명령어

```bash
make build      # bin/kvote 빌드 (ldflags 로 버전 주입)
make test       # go test ./...  — 네트워크 없이 testdata 픽스처로 파서 검증
make fmt        # gofmt -w ./cmd ./internal
make tidy       # go mod tidy
go vet ./...

# 단일 테스트
go test ./internal/nesdc -run TestDetailResults -v
```

라이브 동작 확인:

```bash
./bin/kvote nesdc list -f table          # 여론조사 결과 목록
./bin/kvote nesdc show <nttId>           # 상세 메타
./bin/kvote nesdc pull <nttId> -o /tmp/x # 첨부 다운로드
```

## 아키텍처

```
cmd/kvote/          CLI (cobra). provider 별로 명령 그룹 분리.
  root.go           전역 플래그(--format/--delay/--base-url), 클라이언트 빌더
  nesdc.go          nesdc 명령 그룹 + 렌더링 헬퍼(renderList/renderDetail/renderAgencies)
  nec.go            nec 명령 그룹 (스텁 + roadmap)
internal/nesdc/     NESDC provider — HTML 스크래핑 클라이언트 (package nesdc)
  client.go         rate-limited HTTP + goquery 파서 진입점(getDoc)
  board.go          게시판 레지스트리 (bbsId+menuNo 로 파라미터화)
  list.go           목록 파서 (.row.th 헤더 → .row.tr 행)
  detail.go         view.do 상세 파서 (메타 테이블 + 첨부)
  download.go       FileDown.do 다운로드 + 파일명 인코딩 복구
  agency.go         onvy 조사기관 등록/취소 현황 파서
internal/output/    json / jsonl / table 렌더러 (한글 폭 보정)
internal/version/   ldflags 주입 버전 메타
```

### 핵심 설계 포인트 (코드 읽기 전 알아둘 것)

- **모든 NESDC 게시판은 동일한 eGovFrame 엔진**: `bbs/{bbsId}/list.do` · `view.do` ·
  `cmm/fms/FileDown.do`. 새 게시판 추가는 `board.go` 의 `boards` 맵에 한 줄 추가로 끝.
- **목록 마크업은 `<table>` 이 아님**: 헤더는 `.row.th`, 데이터는 `.row.tr`(`<a>` 또는 `<p>`),
  셀은 `.col`. `nttId` 는 행/중첩 앵커의 href 에서 정규식으로 추출. 헤더 라벨을 그대로 JSON 키로
  사용하므로 게시판마다 컬럼이 자기 설명적.
- **첨부 다운로드의 atchFileId/fileSn 는 이미 percent-encoded**. `DownloadURL` 은 재인코딩하지
  않고 문자열 결합(중요 — `url.Values.Encode()` 쓰면 `%` 가 이중 인코딩됨).
- **Content-Disposition 파일명 인코딩이 두 형태**: percent-encoded UTF-8(+공백) 또는
  raw UTF-8 가 Latin-1 로 잘못 읽힌 형태. `decodeFilename`/`fixLatin1UTF8` 가 둘 다 복구.
- **상세 메타는 lossless**: `Detail.Fields` 는 모든 th/td 행을 순서대로 보존(176행 규모).
  `Detail.Summary` 는 단일 라벨 행 중 분석 핵심 스칼라만 추림(`summaryLabels`). 일부 첨부는
  공표일시 24/48시간 후 공개라 그 전엔 다운로드가 "embargoed" 에러로 표면화됨.
- **rate limit**: `Client.throttle()` 이 요청 간 `--delay`(기본 700ms) 보장. 예의 있는 수집 원칙.

### 테스트 전략

`internal/nesdc/nesdc_test.go` 는 `httptest.Server` 로 `testdata/*.html` 픽스처를 서빙하고
`WithBaseURL` 로 클라이언트를 연결해 **네트워크 없이** 전체 경로(URL 빌드 → 파싱)를 검증합니다.
포털 마크업이 바뀌면 픽스처를 최신 HTML 로 교체하세요.

## NEC provider 작업 시 주의 (준비 중)

- `info.nec.go.kr` (선거통계시스템) 은 **robots.txt 가 `Disallow: /`** — 직접 스크래핑 지양.
- 공식 OpenAPI 는 키 발급 필요 → 키리스 원칙상 후순위.
- 우선순위: **키 없이 받는 공개 데이터 파일**(data.nec.go.kr / data.go.kr CSV·XLS 파일셋).
- NEC 게시판은 NESDC 와 다른 엔진(`www.nec.go.kr/.../List.do?cbIdx=N`) — 별도 파서 필요.

## 컨벤션

- 출처별 provider 는 `internal/<provider>` 패키지로 격리, `cmd/kvote/<provider>.go` 가 명령 노출.
- 새 출력 타입은 `internal/output` 의 `WriteJSON`/`WriteJSONL`/`WriteTable` 재사용.
- 한글 정렬이 필요한 표는 `output.WriteTable`(CJK 더블폭 계산 내장)만 사용.
