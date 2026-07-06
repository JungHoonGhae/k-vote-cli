# Changelog

`kvote` 사용자 관점의 변경 이력입니다. 각 버전에서 "어떤 선거 데이터를 어떻게 받을 수 있게 됐는지"를 정리합니다.

형식은 [Keep a Changelog](https://keepachangelog.com/ko/1.1.0/)를 따르고, 버전은 [SemVer](https://semver.org/lang/ko/)를 따릅니다.

## [Unreleased]

## [0.4.1] - 2026-07-06

### 설치·배포 개선
- **설치 스크립트** — `curl -fsSL .../install.sh | sh` (macOS/Linux) 와 `irm .../install.ps1 | iex` (Windows) 한 줄 설치를 추가했습니다. 다운로드 후 체크섬을 검증합니다.
- **`go install` 버전 표시** — `go install` 로 설치해도 `kvote version` 이 "dev" 대신 실제 버전과 커밋을 표시합니다.
- **Homebrew 안내 정정** — tap 이 Cask 방식이라 macOS 전용임을 문서에 명확히 했습니다 (Linux 는 설치 스크립트 또는 `go install`).
- 릴리즈 노트가 CHANGELOG 의 해당 버전 섹션으로 게시됩니다.

## [0.4.0] - 2026-07-03

여론조사 결과 집계표를 정확히 골라 받습니다. 국정수행 지지율처럼 정규화 마스터가 없고 조사별 집계표 PDF에만 있는 수치를, kvote 가 올바른 PDF를 정확히 surface 해주면 사람·AI 에이전트가 직접 판독합니다. 이질적 PDF 수치를 억지로 파싱해 조용히 틀릴 위험을 피하는 의도적 설계입니다.

### NESDC — 집계표 받기 (API 키 없이)
- **`nesdc tabulation`** — 여론조사의 여러 첨부 중 결과 집계표(통계표) PDF만 정확히 골라 내려받습니다(단건 `<nttId>` / 배치 `--sync --from`). 설문지·질문지는 제외하고, 집계·통계 파일 또는 유일한 비설문 PDF를 선택합니다. kvote 는 파일을 surface 할 뿐 표 안의 수치(국정수행 긍정/부정 등)를 파싱하지 않습니다 — 판독은 사람 또는 AI 에이전트의 몫입니다.

## [0.3.0] - 2026-07-03

성별·연령대별 투표율. 개표결과에는 없는 "누가 투표했는가"의 인구통계 축(성별·연령대·지역)을 API 키 없이 정규화합니다. 여론조사 표본 교차표(`nesdc show --crosstab`)와 같은 축이라, 여론조사·투표율·개표결과를 나란히 비교할 수 있습니다. 원자료 보존 + 정의 명시 파생값이라는 중립 원칙은 그대로입니다.

### NEC — 투표율 (API 키 없이)
- **`nec turnout-analysis`** — data.go.kr "투표율 분석" ZIP을 성별·연령대별·지역별 투표율로 정규화합니다. 개표결과에 없는 인구통계 축이라, 여론조사 교차표·개표결과와 나란히 비교할 수 있습니다.
- **`kvote db ingest turnout`** / MCP **`ingest_turnout`** — 위 데이터를 로컬 DB에 적재해 SQL로 교차 질의합니다. 소스가 보고한 투표율은 원자료로 보존하고, `v_turnout_derived` 가 재계산값(rate_computed)을 나란히 제공합니다.

## [0.2.0] - 2026-07-02

로컬 통합 데이터셋과 AI 에이전트 인터페이스. 수집한 개표결과·여론조사를 하나의 로컬 SQLite로 모아, 사람은 SQL로 질의하고 AI 에이전트는 MCP로 탐색·수집·질의를 한 번에 합니다. 원자료는 그대로 보존하고, 파생값은 정의가 명시된 표준 뷰(투표율·득표율·유효표)만 제공하는 중립 원칙은 그대로입니다.

### MCP — AI 에이전트 인터페이스 (키리스)
- **`kvote mcp`** — kvote를 Model Context Protocol 서버(stdio)로 노출합니다. 에이전트가 연결 하나로 데이터 탐색 → API 키 없이 다운로드 → 로컬 적재 → SQL 질의까지 끝냅니다.
- 도구 5종: `search_datasets`(파일 데이터 검색) · `list_elections`(선거종류별 최신 회차) · `ingest_results`(개표결과 적재) · `ingest_polls`(여론조사 적재) · `query`(read-only SQL).
- 리소스 `kvote://schema` — 테이블·뷰 스키마와 파생값 정의. 에이전트가 질의 전에 읽는 진입점.

### 로컬 데이터셋 (SQLite, 중립)
- **`kvote db ingest results <pk>`** — 개표결과 CSV를 내려받아 투표구 단위로 로컬 DB에 적재합니다(멱등: 같은 데이터셋 재적재 시 교체).
- **`kvote db ingest polls`** — NESDC 누적 여론조사 엑셀을 적재합니다.
- **`kvote db query "<sql>"`** — 로컬 DB에 read-only SQL을 실행합니다. 쓰기 SQL은 엔진이 거부합니다.
- 원자료 테이블(개표결과·후보·여론조사·정당지지)과 표준 파생 뷰(`v_results_derived`·`v_agg_sgg`·`v_agg_sido`·`v_agg_national`)를 제공합니다. 뷰 정의는 기존 `nec results --aggregate`와 동치이며, 동치성 테스트로 정의 드리프트를 막습니다.
- 전역 `--db` 플래그로 DB 경로를 지정할 수 있습니다(기본: OS 설정 디렉터리).

## [0.1.0] - 2026-06-28

첫 공개 릴리즈. 흩어진 한국 선거 공개 데이터(개표결과·여론조사)를 **API 키 없이** 한 명령으로 검색·다운로드·정규화합니다.

### NEC — 개표결과 (중앙선거관리위원회 → data.go.kr 파일 데이터, 키리스)
- **`nec corpus`** — 역대 핵심 선거 9종(대선·총선·비례·지방)의 개표결과를 동시 다운로드합니다. `--normalize` 로 투표구별 정규화 JSONL까지 한 번에.
- **`nec datasets`** — 선관위 공개 파일 데이터(개표결과·투표율 등)를 검색합니다.
- **`nec latest`** — 선거종류의 최신 회차 데이터셋을 두 소스에서 자동 해석합니다.
- **`nec pull`** — 파일 데이터(CSV/XLSX) 원본을 그대로 받습니다.
- **`nec results`** — 개표결과 CSV(EUC-KR)를 투표구별 정규화 레코드로 변환합니다. 원자료를 보존하면서 정의가 명시된 표준 파생값(투표율·득표율·유효표)만 더합니다.

### NESDC — 여론조사 (중앙선거여론조사심의위원회, 키리스)
- **`nesdc sync`** — 기간/조건 전체를 페이지네이션하며 JSONL로 일괄 수집합니다. "이번주 여론조사 전수"가 한 명령.
- **`nesdc bulk`** — 주차별 누적 마스터 엑셀을 정규화된 여론조사 레코드로 출력합니다.
- **`nesdc show` / `nesdc pull`** — 단건 상세 메타 조회 및 첨부(통계표·설문지) 다운로드.

### data.go.kr OpenAPI (실험적 — 인증키 필요)
- **`nec turnout` / `nec winners` / `nec elections`** — 투표율·당선인·선거코드를 data.go.kr OpenAPI로 조회합니다(`KVOTE_DATAGOKR_KEY`).
- **`api login` / `list` / `apply` / `config`** — 브라우저 세션으로 data.go.kr에 로그인해, 필요한 OpenAPI 활용신청을 확인·신청합니다. 키 입력은 사용자가 브라우저에서 직접 하며 kvote는 비밀번호를 보지 않습니다.

### 도구
- **`kvote doctor`** — 킬러 경로(검색→다운로드→정규화)를 라이브로 점검하는 스모크 테스트.
- 모든 출력은 `--format json | jsonl | table`. 한글 폭 보정 테이블 내장.

### 설계 원칙
- **키리스**: 핵심 기능(개표결과·여론조사)은 API 키 발급 없이 동작합니다.
- **중립**: kvote는 어떤 분석적 입장도 취하지 않습니다. 원자료를 완전 보존하고, 정의가 명시된 재현 가능한 표준 파생값만 더합니다. 플래그·점수·"이상치" 판단은 제공하지 않습니다.
- **예의 있는 수집**: 요청 간 `--delay`(기본 700ms)를 보장하고, 탐지 회피로 차단된 소스를 우회하지 않습니다.

### 배포
- macOS·Linux·Windows (amd64·arm64) 바이너리. Homebrew tap(`JungHoonGhae/k-vote-cli`) 제공.

[Unreleased]: https://github.com/JungHoonGhae/k-vote-cli/compare/v0.4.1...HEAD
[0.4.1]: https://github.com/JungHoonGhae/k-vote-cli/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/JungHoonGhae/k-vote-cli/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/JungHoonGhae/k-vote-cli/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/JungHoonGhae/k-vote-cli/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/JungHoonGhae/k-vote-cli/releases/tag/v0.1.0
