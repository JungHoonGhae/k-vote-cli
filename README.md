<p align="right"><strong>한국어</strong> · <a href="README.en.md">English</a></p>

<div align="center">
  <h1>k-vote-cli</h1>
  <p><strong>개표결과도, 이번주 여론조사도. 누구나 한 명령으로 받아본다.</strong></p>
  <p>한국 선거 공개 데이터를, 특별한 권한이나 가입 없이. 사람이든 AI든 같은 명령(<code>kvote</code>)으로 받아 바로 봅니다.</p>
  <p><sub>역대 개표결과 <strong>투표구별 정리</strong>, <strong>이번주 여론조사 전수 수집</strong>, 정당지지율 시계열까지. <a href="#기능">전체 기능표 ↓</a></sub></p>
</div>

<p align="center">
  <a href="#quick-start"><strong>Quick Start</strong></a> ·
  <a href="#기능"><strong>기능</strong></a> ·
  <a href="#설치"><strong>설치</strong></a> ·
  <a href="#이걸로-검증할-수-있는-것"><strong>검증 예시</strong></a> ·
  <a href="CLAUDE.md"><strong>내부 구조</strong></a>
</p>

<p align="center">
  <a href="docs/k-vote-cli-promo.mp4"><img src="docs/k-vote-cli-promo.gif" alt="k-vote-cli 홍보: 개표결과도, 이번주 여론조사도 한 명령으로" width="760" /></a>
</p>
<p align="center"><sub><a href="docs/k-vote-cli-promo.mp4">HD 영상</a> · <a href="docs/k-vote-cli.gif">터미널 데모</a></sub></p>

> [!WARNING]
> 공개 포털의 HTML 구조에 의존하므로 사이트 개편 시 동작이 깨질 수 있습니다.
> 수집 대상은 **법적 공개 의무가 있는 공공 데이터**(선거여론조사기준에 따른 등록·공개 자료)이며,
> 기본 rate limit 을 적용해 예의 있게 수집합니다. 본인 책임 하에 사용하세요.

## 왜 만들었나

선거관리에 대한 불신이 어느 때보다 높아진 시대입니다. 닿을 수 없는 것은 미지로 남고,
미지의 영역에는 두려움과 불신이 싹트기 마련입니다. 무엇이든 AI와 함께 분석할 수 있는 시대인데도
선거 데이터만은 막힌 포털과 낡은 형식, 데이터마다 일일이 신청해야 하는 인증키 뒤에 있습니다 —
사람에게도, AI에게도 닿기 어려운 곳에.

이 도구가 불신이라는 물음에 답할 수는 없습니다. 다만 개발자의 한 사람으로서 이 문제를 해결하고
싶었습니다 — 충분하지 않더라도, 일단 **이미 있는 데이터부터** 조금 더 닿기 쉽도록. 그리고 더
많은 사람이 직접 열어 보고 검증할 수 있는 열린 형태일수록, 조금 더 신뢰할 만한 시스템을 같이
만들어갈 수 있지 않을까 — 그런 막연한 생각에서 시작한 프로젝트입니다.

선거 결과를 직접 확인하고 싶을 때, 이번주 여론조사가 어떻게 나왔는지 궁금할 때.
분명 "공개돼 있다"는데, 막상 받으려 하면 **어디서·무엇을·어떤 파일로** 받아야 할지부터 막막합니다.
info.nec.go.kr 은 막혀 있고, data.go.kr 엔 비슷한 데이터셋이 수십 개, 어렵게 받은 파일은 글자가
깨지거나 열리지 않죠. 여론조사는 API 조차 없어 글마다 PDF 를 받아 표를 손으로 읽어야 합니다.
개발자라고 덜 막막한 것도 아니고, 비전문가에겐 아예 넘기 힘든 벽이 됩니다.

`k-vote-cli` 는 그 막막함을 없앱니다. 어디서·뭘·어떤 파일인지 몰라도 됩니다. 한 명령이면 개표결과도
이번주 여론조사도, kvote 가 **필요한 자료를 알아서 찾아** 받자마자 분석할 수 있게 정리된 형태로
내려줍니다. 사람이 치든 AI 가 호출하든 똑같이.

<details>
<summary>기술적으로 무엇이 막혀 있었나</summary>

- 선거통계시스템(info.nec.go.kr)은 자동 접근을 전면 차단(robots)
- 파일은 옛 인코딩(EUC-KR)·제각각인 엑셀 양식·중복된 투표구
- data.go.kr OpenAPI 는 가입·활용신청·인증키 발급 절차

k-vote-cli 가 이걸 전부 흡수합니다.
</details>

> **중립.** 이 프로젝트는 어떤 주장도 하지 않습니다. 어떤 질문을 들고 왔든, 어떤 결론에 이르든,
> 누구나 같은 데이터에서 같은 방식으로 출발할 수 있게 하는 것 — 그것이 전부입니다.
> 원본을 그대로 보존하고, 정의가 분명한 표준 계산값(투표율·득표율·합계)까지만 더합니다.
> 판단은 데이터를 받은 사람의 몫입니다.

## 무엇이 특별한가

- **역대 개표결과가 한 명령에** — 대선·총선·지방을 동시에 받아 투표구별로 정리 (`nec corpus`)
- **이번주 여론조사까지 통째로** — 등록된 모든 조사와 정당지지율 시계열 (`nesdc sync` · `bulk`)
- **인증키 발급을 AI가 대신** — data.go.kr 가입·활용신청·승인을 알아서 처리 (`api login` · `apply`)
- **원본 그대로, 언제 돌려도 같은 결과** — 누구나 그대로 재현

## Before → After

| 데이터 | 기존 | k-vote-cli |
|---|---|---|
| **개표결과** (투표구별) | info.nec.go.kr 차단 → data.go.kr 수동 검색 → CSV(EUC-KR) 다운 → 인코딩 변환 → long-format 수작업 파싱·중복 처리 | `kvote nec results <pk>` |
| **역대 핵심 개표결과** | 위 과정을 선거마다 반복 | `kvote nec corpus --normalize` |
| **여론조사** | 게시판 클릭 → 글마다 PDF 다운 → 표 일일이 판독 *(공식 API 없음)* | `kvote nesdc sync` |
| **투표율·당선인** (OpenAPI) | 회원가입 → 활용신청·승인 대기 → 인증키 발급 → 코드 찾기 → XML 호출·페이징·파싱 | `kvote api login` *(1회)* → `kvote nec winners <sgId>` |

## 기능

> 출력은 `-f json|jsonl|table`. **API 키 없이** 열이 **O** 인 명령은 가입·인증키 발급 없이 바로 동작합니다.
> **X** = data.go.kr 인증키 필요(`kvote api` 로 자동 발급). OpenAPI·계정 그룹은 실험적 기능입니다.

| 분류 | 기능 | 커맨드 | API 키 없이 |
|---|---|---|:--:|
| **핵심** | 역대 핵심 개표결과 동시 다운로드 + 투표구별 정규화 | `nec corpus --normalize` | **O** |
| 개표결과 | 개표결과를 투표구별로 정규화 (집계·본/사전 분리) | `nec results <pk>` | **O** |
| 개표결과 | 공개 데이터셋 검색 (개표결과 등) | `nec datasets -q <검색어>` | **O** |
| 개표결과 | 선거종류 최신 회차 자동 해석 | `nec latest <키워드>` | **O** |
| 개표결과 | 원본 파일 다운로드 (CSV/XLSX 자동) | `nec pull <pk>` | **O** |
| 투표율 | 투표율 분석(ZIP)을 성별·연령대별·지역별로 정규화 | `nec turnout-analysis <pk>` | **O** |
| 여론조사 | 기간/조건 전체를 JSONL 로 전수 수집 | `nesdc sync` | **O** |
| 여론조사 | 누적 마스터 엑셀 → 정당지지율 (2023.10.30~ 1,400+건) | `nesdc bulk` | **O** |
| 여론조사 | 단건 상세 메타 + 표본 구성 교차표 | `nesdc show <nttId> --crosstab` | **O** |
| 여론조사 | 첨부 PDF(통계표·설문지) 다운로드 | `nesdc pull <nttId>` | **O** |
| 여론조사 | 결과 집계표 PDF만 정확히 골라 받기 (단건·`--sync` 배치) | `nesdc tabulation <nttId>` | **O** |
| 로컬 DB | 개표·여론조사·투표율을 SQLite 에 적재 + read-only SQL | `db ingest …` · `db query` | **O** |
| AI 에이전트 | MCP 서버 (탐색·수집·SQL 질의를 tool 로) | `mcp` | **O** |
| 점검 | 핵심 경로 라이브 점검 (사이트 개편 깨짐 감지) | `doctor` | **O** |
| OpenAPI | 투표율 (시도/구시군별, 본/사전 분리) | `nec turnout <sgId> --sgtype N` | X |
| OpenAPI | 당선인 (선거구·기호·정당·이름·득표) | `nec winners <sgId> --sgtype N` | X |
| OpenAPI | 선거코드 레지스트리 (1987~ 모든 sgId·선거종류) | `nec elections` | X |
| 계정 | 브라우저 1회 로그인 → 세션 유지 | `api login` | **O** |
| 계정 | 내 활용신청 현황 (상태·**만료예정일**) | `api list` | X |
| 계정 | OpenAPI 활용신청 자동제출 (목적 필수·확인) | `api apply <pk> --purpose` | X |

<details>
<summary><strong>주요 옵션 펼치기</strong></summary>

| 커맨드 | 핵심 옵션 |
|---|---|
| `nec corpus` | `--normalize` 받는 즉시 투표구별 JSONL · `-o` 저장 위치 · `--concurrency` |
| `nec results` | `--file` 로컬 파싱 · `--aggregate {town\|sgg\|sido\|national}` · `--by-votetype` · `--race`/`--leaf-only`(XLSX) |
| `nec datasets`/`pull` | `--source datagokr\|openportal` (개방포털: data.nec.go.kr, dataId 사용) |
| `nec turnout`/`winners` | `--sgtype` 1=대통령 2=국회의원 3=시도지사 4=구시군장 5=시도의원 6=구시군의원 · `--api-key`(기본 `KVOTE_DATAGOKR_KEY`) |
| `nesdc sync` | `-q`·`--field`·`--from`/`--to`·`--date-field`·`--gubun`·`--max-pages`·`--pull` |
| `api apply` | `--purpose`(필수) · `--category research\|web\|app\|ref\|etc` · `--yes` |
| `api config` | `--auto-apply true` (확인 없이 바로 신청) |

</details>

### `nec corpus` — 대표 기능

`nec corpus --normalize` 한 줄이면 역대 핵심 개표결과(대선 제16·17·21대, 총선·비례, 지방 5~8회)가
동시 다운로드되고, 받는 즉시 투표구별 JSONL 로 정규화됩니다. 받아서 바로 들여다볼 수 있는 상태로.

```bash
kvote nec corpus --normalize -o ./corpus
duckdb -c "SELECT * FROM read_json_auto('./corpus/*.jsonl') LIMIT 5"
```

### 정규화가 주는 것

| 차원 | 내용 |
|---|---|
| 투표유형 | 행마다 본투표 / 관내사전 / 관외사전 / 거소로 분류 |
| 다단계 집계 | 투표구 → 읍면동 → 선거구 → 시도 → 전국 (지표 합산 + 투표율) |
| 파생값 | 투표율(투표/선거인) · 유효투표수(투표−무효) · 후보 득표율(득표/유효), 모두 정의 명시 |
| 원자료 보존 | 선거인수·투표수·무효·기권·후보별 득표 전부 보존, 어떤 항등식도 직접 계산 가능 |
| 표본 구성 | 여론조사 성별·연령·지역 × 완료·가중 사례수 + 가중방법 |

### 이걸로 검증할 수 있는 것

`AGENTS.md` 에 에이전트용 레시피가 더 있습니다.

| 검증 | 방법 |
|---|---|
| 사전투표 vs 본투표 득표율 | `nec results --aggregate sgg --by-votetype` |
| 개표 항등식 (투표수 = 후보합 + 무효) | `nec results` → 원자료로 직접 검산 |
| 여론조사 표본 대표성·가중 | `nesdc show --crosstab` |
| 기관×방식별 여론조사 분포 | `nesdc sync` → 전수 집계 |
| 정당지지율 시계열 | `nesdc bulk` |
| 성별·연령대별 투표율 | `nec turnout-analysis <pk>` — 여론조사 표본 교차표(`nesdc show --crosstab`)와 같은 축(성별·연령)이라 여론조사·투표율·개표결과를 나란히 비교할 수 있습니다. |

## 지원하는 선거 데이터

kvote 는 특정 선거를 하드코딩하지 않습니다 — **선관위가 포털에 올리는 순간 자동으로 대상이 됩니다**
(`nec datasets`/`nec latest` 로 언제든 최신 목록 확인). 아래는 현재 포털에 공개된 범위입니다.

| 선거 | 받을 수 있는 회차 | 형식 | 비고 |
|---|---|---|---|
| 대통령선거 | 제16 · 17 · 21대 | CSV | 제18~20대는 포털에 파일 데이터가 없음 |
| 국회의원선거 | 제22대 (지역구 + 비례대표) | CSV | |
| 전국동시지방선거 | 제5 ~ 8회 | XLSX | 시·도지사, 교육감 등 7개 선거 동시 포함 |
| 재·보궐선거 | 통합 파일 | XLSX | |
| 투표율 분석 (성별·연령) | XLSX 제공 회차 (제20·21대 대선, 제21·22대 총선, 제8회 지선 등) | ZIP | PDF 만 있는 옛 회차는 원본만 (`nec pull`) |
| 여론조사 | 게시판 전체 + 정당지지도 누적 (2023-10-30 ~) | HTML/XLSX/PDF | |

**헷갈리기 쉬운 것들**

- **번호 체계가 선거마다 다릅니다.** 대통령·국회의원선거는 "제N**대**"(제21대 대선 = 2025,
  제22대 총선 = 2024), 지방선거는 "제N**회**"(제8회 = 2022, 제9회 = 2026). "제19대"라는 번호는
  대선(2017)·총선(2012)에는 있지만 지방선거에는 없는 번호입니다.
- **막 끝난 선거는 바로 없습니다.** 선관위가 파일 데이터를 정리해 포털에 게시하기까지 시차가
  있습니다(예: 2026-06 제9회 지방선거는 아직 미게시). 게시되는 즉시 같은 명령으로 받아집니다 —
  `kvote nec latest "지방선거 개표결과"` 가 최신 회차를 자동으로 찾아줍니다.
- **하나의 지방선거 파일 안에 여러 선거가 들어 있습니다.** 시·도지사, 구·시·군의 장, 교육감,
  광역·기초의원, 비례가 시트로 나뉘어 있고, `nec results --race 시·도지사` 처럼 골라 꺼냅니다.
- **투표율은 두 가지가 다른 데이터입니다.** 개표결과에서 파생되는 투표율(투표수/선거인수)과 달리,
  "투표율 분석"(`nec turnout-analysis`)은 **누가 투표했는가**(성별·연령대별) — 개표결과에는 없는
  별도 축입니다.

## 설치

**`go install` (권장)**

```bash
go install github.com/JungHoonGhae/k-vote-cli/cmd/kvote@latest
```

**Homebrew (macOS/Linux)**

```bash
brew install JungHoonGhae/k-vote-cli/kvote
```

**소스 빌드**

```bash
git clone https://github.com/JungHoonGhae/k-vote-cli
cd k-vote-cli
make build        # -> bin/kvote
```

> 크로스플랫폼 바이너리(macOS/Linux/Windows · arm64/amd64)는 태그(`vX.Y.Z`) 푸시 시
> [GitHub Releases](https://github.com/JungHoonGhae/k-vote-cli/releases) 에 goreleaser 로 자동 게시됩니다.

## Quick Start

### For Agent

```text
k-vote-cli 설치: go install github.com/JungHoonGhae/k-vote-cli/cmd/kvote@latest

`kvote doctor` 로 핵심 경로가 살아있는지 먼저 확인.
개표결과는 API 키 없이 바로 받을 수 있다:
  kvote nec corpus --normalize -o ./corpus   # 역대 핵심 개표결과 → 투표구별 JSONL
  kvote nec results <pk> -f jsonl             # 단일 데이터셋 정규화
출력은 -f jsonl 로 받아 jq/duckdb 로 질의.
OpenAPI(turnout/winners)는 키가 필요하나 투표율·당선인은 results 로도 파생 가능.
```

### For Human

```bash
# 핵심: 역대 개표결과를 한 명령으로
kvote nec corpus --normalize -o ./corpus
duckdb -c "SELECT * FROM read_json_auto('./corpus/*.jsonl') LIMIT 5"

# 단일 데이터셋: 검색 → 정규화
kvote nec datasets -q 개표결과 -f table
kvote nec results 15025527 -f jsonl > votes.jsonl     # 투표구별, 후보 득표 포함
kvote nec results 15025527 --aggregate sgg --by-votetype -f jsonl   # 선거구×투표유형

# 여론조사: 전수 수집
kvote nesdc sync --from 2026-01-01 > surveys.jsonl
kvote nesdc show 19366 --crosstab -f table            # 단건 상세 + 표본 구성
kvote nesdc bulk -f jsonl > polls.jsonl               # 정당지지율 누적

# data.go.kr OpenAPI: 키 발급부터 호출까지 (실험적)
kvote api login                                       # 브라우저 1회 로그인
kvote api apply 15000900 --purpose "선거 데이터 분석 연구"
export KVOTE_DATAGOKR_KEY=<일반 인증키>
kvote nec winners 20240410 --sgtype 2 -f jsonl        # 제22대 총선 당선인 254명
```

## MCP 서버 (AI 에이전트용)

`kvote mcp` 는 stdio 로 [Model Context Protocol](https://modelcontextprotocol.io) 서버를 띄웁니다.
에이전트가 셸 명령 대신 tool 호출로 같은 데이터에 접근할 수 있습니다.

### 왜 로컬 DB에 적재하나?

로컬 DB는 **필수가 아니라 선택**입니다. 단발성 분석이면 `nec results`/`nesdc sync` 로 JSONL을 받아
`jq`·`duckdb` 로 바로 처리하는 게 더 빠릅니다. 아래가 필요할 때 적재가 값을 냅니다.

- **교차 질의**: 여러 선거·개표결과·여론조사를 한 곳에 모아 SQL로 조인·집계합니다. JSONL 파일 여러 개를
  매번 맞춰 다루는 것보다 강력합니다.
- **반복 재사용**: 투표구 수십만 행을 한 번만 내려받아 정규화해 두면, 이후 질의는 다운로드·파싱 없이
  즉시 실행됩니다.
- **표준 파생값을 SELECT 한 줄로**: 투표율·득표율·유효표·다단계 집계가 뷰(`v_results_derived`·`v_agg_*`)로
  미리 정의돼 있어, 매번 산식을 다시 짤 필요가 없습니다. 정의는 뷰 SQL에 명시(중립) — `kvote://schema` 참조.
- **에이전트 친화**: MCP `query` 로 에이전트가 자연어를 SQL로 옮겨 스스로 슬라이스합니다. 셸 파이프라인을
  조립하거나 파일을 관리할 필요가 없습니다.

원자료는 그대로 저장되고, DB는 접근·질의 편의를 더할 뿐 **어떤 판단도 내리지 않습니다.**

모든 tool 은 API 키 없이 동작합니다.

| tool / 리소스 | 내용 |
|---|---|
| `search_datasets` | data.go.kr 개표결과 파일 데이터셋 키워드 검색 |
| `list_elections` | 선거종류 키워드로 최신 회차 데이터셋 조회 |
| `ingest_results` | 개표결과를 내려받아 로컬 SQLite에 적재 (멱등) |
| `ingest_polls` | NESDC 누적 여론조사 엑셀을 내려받아 적재 (멱등) |
| `ingest_turnout` | 투표율 분석(성별·연령대별)을 내려받아 적재 (멱등) |
| `query` | 로컬 DB에 read-only SQL 질의 |
| `kvote://schema` | 테이블·뷰 스키마 + 파생값 정의 (리소스, `query` 전에 먼저 읽기) |

원자료는 그대로 저장되고, 파생값(투표율·득표율·유효표 등)은 뷰 SQL 정의로만 존재합니다.
`query` 는 read-only 연결이라 쓰기 SQL은 엔진이 거부합니다 — 판단은 여전히 에이전트/사람의 몫입니다.

에이전트 등록 예시 (Claude Code):

```bash
claude mcp add kvote -- kvote mcp
```

또는 설정 파일에 직접 등록:

```json
{
  "mcpServers": {
    "kvote": { "command": "kvote", "args": ["mcp"] }
  }
}
```

### `kvote db` (CLI에서 직접 SQLite 다루기)

MCP 없이도 같은 로컬 DB를 CLI로 적재·질의할 수 있습니다.

```bash
kvote db ingest results <publicDataPk>   # 개표결과 CSV → 적재 (멱등)
kvote db ingest polls                     # NESDC 누적 여론조사 엑셀 → 적재 (멱등)
kvote db ingest turnout <publicDataPk>    # 투표율 분석(성별·연령대) → 적재 (멱등)
kvote db query "SELECT * FROM v_agg_sgg LIMIT 5" -f table
```

DB 경로는 기본적으로 OS 설정 디렉터리(`.../kvote/kvote.db`)이며 전역 플래그 `--db` 로 재정의합니다.

### 필터 (`nesdc sync`)

| 플래그 | 의미 | 값 |
|---|---|---|
| `-q`, `--query` | 검색어 | 자유 문자열 |
| `--field` | 검색 대상 필드 | `agency` `client` `method` `frame` `name` `sido` `regno` |
| `--from` / `--to` | 기간 | `YYYY-MM-DD` |
| `--date-field` | 기간 기준 | `registered`(기본) `published` `surveyed` |
| `--gubun` | 선거구분 | 선거구분 코드 (예: `VT044` 제22대 대선) |

> 포털은 `--date-field` 없이는 기간 필터를 무시합니다. 기간 지정 시 자동으로 `registered` 를 기본 적용합니다.

### 게시판 (`[board]` 인자)

| name | 내용 |
|---|---|
| `results` | 여론조사결과 보기 (상세 메타 + PDF), 기본값 |
| `data` | 여론조사결과 주요 데이터 (주차별 벌크) |
| `notices` · `library` · `actions` · `violations` | 공지·자료실·조치현황·위반사례 |

## 출력 형식 · 전역 플래그

| `-f` | 용도 |
|---|---|
| `json` | 기본. 사람이 읽거나 `jq` 로 가공 |
| `jsonl` | 한 줄에 한 레코드. 대량 수집/스트리밍 (jq·duckdb·pandas 바로) |
| `table` | 터미널에서 바로 보기 (한글 폭 정렬) |

| 전역 플래그 | 기본값 | 설명 |
|---|---|---|
| `-f, --format` | `json` | 출력 형식 |
| `--delay` | `700ms` | 요청 간 최소 간격 (rate limit) |
| `--base-url` | - | 포털 base URL 재정의 (테스트용) |
| `--db` | OS 설정 디렉터리 | 로컬 SQLite DB 경로 (`kvote mcp`/`kvote db` 가 사용) |

## 데이터 출처

| provider | 사이트 | 내용 | API 키 없이 |
|---|---|---|:--:|
| `nec` | 중앙선거관리위원회 → data.go.kr | 개표결과(파일) + 투표율·당선인(OpenAPI) | 파일 **O** / OpenAPI X |
| `nesdc` | 중앙선거여론조사심의위원회 (nesdc.go.kr) | 여론조사 결과·표본 구성 | **O** |
| `api` | data.go.kr 계정 | OpenAPI 활용신청·인증키·만료 관리 | X |

`nec` 은 선거통계시스템(info.nec.go.kr)이 robots.txt 로 전면 크롤링을 금지하므로 **직접 스크래핑하지
않습니다.** 대신 선관위가 공식 배포 채널인 **data.go.kr 의 개표결과 파일(CSV/XLSX)** 을 API 키 없이
검색·다운로드합니다. `nesdc.go.kr` 은 공식 API 가 없어 스크래핑이 유일한 프로그램적 접근입니다.

## 개발

```bash
make build    # 빌드
make test     # 테스트 (네트워크 불필요, testdata 픽스처)
make fmt      # gofmt
go vet ./...
```

내부 구조는 [CLAUDE.md](CLAUDE.md), 에이전트 레시피는 [AGENTS.md](AGENTS.md) 참고.

## 기여

이슈·PR 환영합니다. 시작 전 [CONTRIBUTING.md](CONTRIBUTING.md)의 **두 가지 타협 불가 원칙**(중립성 · API 키 없이)을 읽어 주세요.

- 변경 이력: [CHANGELOG.md](CHANGELOG.md)
- 행동 강령: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- 보안 신고: [SECURITY.md](SECURITY.md) (공개 이슈로 올리지 마세요)

## 라이선스

[MIT](LICENSE)
