<div align="center">
  <h1>kvote</h1>
  <p><strong>한국 선거 공개 데이터에 누구나·AI 에이전트가 접근할 수 있게 하는 비공식 CLI.</strong></p>
  <p>API 키 발급 없이 — 여론조사(NESDC)·개표결과(NEC)를 구조화(JSON/JSONL)로 가져옵니다.</p>
</div>

## 왜 만드나 (취지)

선거관리위원회의 투명성에 대한 사회적 의구심이 커지는 가운데, **공개된 선거 데이터에 누구나
쉽게 접근해 스스로 확인할 수 있는 인프라**가 필요하다. 기존엔 이 데이터가 robots 차단·JSF
포털·PDF·복잡한 클릭 흐름 뒤에 흩어져 사실상 일반 시민·연구자·AI 에이전트가 접근하기
어려웠다. kvote 의 목적은 단 하나 — **그 접근성을 끌어올리는 것.** 흩어진 공개 데이터를
키 없이 한 번의 명령으로, AI 에이전트가 그대로 질의할 수 있는 구조화된 형태로 만든다.

> **중립성 원칙.** kvote 는 어떤 분석적 입장도 취하지 않는다. 우리는 데이터 접근성만 높이며,
> 무엇이 정상·비정상·수상한지 **판단하지 않는다.** 분석에 필요한 파라미터·차원(원자료 보존 +
> 정의가 명확한 표준 파생값)을 광범위하고 중립적으로 제공할 뿐이고, 플래그·점수·"이상치"·
> 해석은 전적으로 소비자(사람 또는 AI 에이전트)의 몫이다.

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8.svg" alt="Go" />
  <img src="https://img.shields.io/badge/output-json%20%7C%20jsonl%20%7C%20table-informational" alt="output formats" />
  <img src="https://img.shields.io/badge/license-MIT-yellow.svg" alt="MIT" />
</p>

> [!WARNING]
> 비공식 도구입니다. 공개 포털의 HTML 구조에 의존하므로 사이트 개편 시 동작이 깨질 수 있습니다.
> 수집 대상은 **법적 공개 의무가 있는 공공 데이터**(선거여론조사기준에 따른 등록·공개 자료)이며,
> 기본 rate limit 을 적용해 예의 있게 수집합니다. 본인 책임 하에 사용하세요.

## 데이터 출처 (provider)

| provider | 사이트 | 내용 | 상태 |
|---|---|---|---|
| `nesdc` | 중앙선거여론조사심의위원회 (nesdc.go.kr) | 여론조사 결과·조사기관 현황 | ✅ |
| `nec` | 중앙선거관리위원회 → data.go.kr | 개표결과·투표율 등 공개 파일 데이터 | ✅ |

`nesdc.go.kr` 은 공식 API 가 없어 스크래핑이 유일한 프로그램적 접근입니다.
`nec` 는 선거통계시스템(info.nec.go.kr)이 robots.txt 로 전면 크롤링을 금지하므로 **직접
스크래핑하지 않습니다.** 대신 선관위가 공식 배포 채널인 **data.go.kr 에 올린 개표결과·투표율
파일 데이터(CSV/XLSX)** 를 — API 키 없이 — 검색·다운로드합니다.

## 기능 — 무엇을 할 수 있나

### 명령어 (전부 키 없이, 출력 `-f json|jsonl|table`)

#### `nesdc` — 여론조사 (중앙선거여론조사심의위원회)

| 명령 | 하는 일 | 핵심 옵션 |
|---|---|---|
| `nesdc boards` | 수집 가능한 6개 게시판 목록 | — |
| `nesdc list [board]` | 게시판 목록 (여론조사 등록현황 등) | `-q` 검색 · `--field`(조사기관/의뢰자/방식…) · `--from`/`--to` · `--date-field`(등록일/공표일/조사일) · `--gubun`(선거구분) |
| `nesdc show <nttId>` | 단건 상세 메타 (기관·방식·표본·응답률·표본오차 + 전체 176행 필드) | `--crosstab`(표본 구성: 성별·연령·지역 × 완료·가중) |
| `nesdc pull <nttId>` | 첨부 PDF(통계표·설문지) 다운로드 | `-o` 저장 위치 |
| `nesdc sync [board]` | 기간/조건 전체를 JSONL 로 일괄 수집 | `--from`/`--to`/`--max-pages`/`--pull` |
| `nesdc bulk` | 주차별 누적 마스터 엑셀 → 정규화 정당지지율 (2023.10.30~ 전체 1,400+건) | `--save` 원본 보존 |
| `nesdc elections` | 선거구분 코드(`--gubun` 값) 실시간 조회 | — |
| `nesdc agencies` | 조사기관 등록현황 | `--cancelled` 취소현황 |

#### `nec` — 개표결과 (중앙선거관리위원회 → data.go.kr, 키 불필요)

| 명령 | 하는 일 | 핵심 옵션 |
|---|---|---|
| `nec datasets` | 선관위 공개 파일 데이터 검색 (개표결과 등) | `-q` 검색어 |
| `nec pull <pk>` | 개표결과 원본 다운로드 (CSV/XLSX 자동) | `-o` 저장 위치 |
| `nec results <pk>` | 개표결과를 **투표구별로 정규화** (CSV=총선·대선, XLSX=지방선거 멀티시트) | `--file` 로컬 파싱 · `--aggregate {town\|sgg\|sido\|national}` 다단계 집계 · `--by-votetype` 투표유형 분리 · `--race`/`--leaf-only`(XLSX) |

### 정규화가 주는 것 (중립 파라미터)

| 분석 차원 | 내용 |
|---|---|
| 투표유형 | 본투표 / 관내사전 / 관외사전 / 거소 — 행마다 분류 |
| 다단계 집계 | 투표구 → 읍면동 → 선거구 → 시도 → 전국 (지표 합산 + 투표율) |
| 파생값 | 투표율(투표/선거인) · 유효투표수(투표−무효) · 후보 득표율(득표/유효) — 정의 명시 |
| 원자료 보존 | 선거인수·투표수·무효·기권·후보별 득표 전부 — 어떤 항등식도 직접 계산 가능 |
| 표본 구성 | 여론조사 성별·연령·지역 × 완료·가중 사례수 + 가중방법 |

### 이걸로 검증할 수 있는 것 (예시 — `AGENTS.md` 레시피)

| 검증 | 방법 |
|---|---|
| 사전투표 vs 본투표 득표율 격차 | `nec results --aggregate sgg --by-votetype` |
| 개표 항등식 (투표수 = 후보합 + 무효) | `nec results` → 원자료로 직접 검산 |
| 여론조사 표본 대표성·가중 강도 | `nesdc show --crosstab` |
| 기관×방식별 여론조사 분포 | `nesdc sync` → 전수 집계 |
| 정당지지율 시계열 | `nesdc bulk` |

> kvote 는 **데이터만 제공**한다 — "정상/이상" 판단은 하지 않는다. 위 검증의 해석은 사용자/AI 에이전트의 몫이다(중립성 원칙).

## 설치

```bash
git clone https://github.com/JungHoonGhae/kvote
cd kvote
make build        # -> bin/kvote
# 또는
make install      # $GOBIN 에 설치
```

## Quick Start

```bash
# 여론조사 결과 최신 목록 (표 형식)
kvote nesdc list -f table

# 검색·기간 필터 (--field 로 검색 대상 지정, --date-field 로 기간 기준 지정)
kvote nesdc list -q 리얼미터 --field agency -f table
kvote nesdc list --from 2025-01-01 --to 2025-01-31 --date-field registered -f table
kvote nesdc list --gubun VT044 -f table                  # 제22대 대통령선거만

# 선거구분 코드(--gubun 값) 실시간 조회
kvote nesdc elections -f table

# 단건 상세 메타데이터 (기관·방식·표본·응답률·표본오차·공표일시 + 교차표)
kvote nesdc show 19366
kvote nesdc show 19366 --crosstab -f table   # 표본 구성(성별·연령·지역 × 완료·가중)

# 첨부파일(통계표·설문지 PDF) 다운로드
kvote nesdc pull 19366 -o ./downloads

# 주차별 누적 마스터 엑셀 → 정규화된 여론조사 레코드 (정당지지율 포함)
kvote nesdc bulk -f jsonl > polls.jsonl                   # 2023.10.30~ 전체 누적
kvote nesdc bulk --save ./archive -f json                # 원본 엑셀도 보존

# 조사기관 등록현황 / 취소현황
kvote nesdc agencies -f table
kvote nesdc agencies --cancelled -f table

# 전체 일괄 수집 → JSONL (대규모 분석용)
kvote nesdc sync --from 2026-01-01 > surveys.jsonl
kvote nesdc sync --max-pages 5 --pull -o ./archive   # 메타 + 첨부 동시 수집

# --- NEC: 선관위 개표결과·투표율 (data.go.kr 공개 파일, 키 불필요) ---
kvote nec datasets -q 개표결과 -f table              # 선관위 공개 데이터셋 검색
kvote nec pull 15025527 -o ./downloads               # 제22대 총선 개표결과 CSV 원본
kvote nec results 15025527 -f jsonl > votes.jsonl    # 투표구별 정규화 (후보 득표 포함)
kvote nec results --file ./downloads/*.csv -f jsonl  # 이미 받은 CSV 파싱 (재다운로드 X)
# 집계 뷰 (중립 파라미터 — 비교·판단은 소비자/AI 에이전트가)
kvote nec results 15025527 --aggregate sgg --by-votetype -f jsonl   # 선거구×투표유형
kvote nec results 15025527 --aggregate sido -f table                # 시도별 투표율
# XLSX 지방선거 개표결과 — 공통 스키마(선거종류·차원 라벨 보존)
kvote nec pull 15101509 -o ./downloads                       # 제8회 지방선거 XLSX 원본
kvote nec results --file ./downloads/*.xlsx --race 교육감 --leaf-only -f jsonl
```

### 필터 (results 게시판)

| 플래그 | 의미 | 값 |
|---|---|---|
| `-q`, `--query` | 검색어 | 자유 문자열 |
| `--field` | 검색 대상 필드 | `agency` `client` `method` `frame` `name` `sido` `regno` |
| `--from` / `--to` | 기간 | `YYYY-MM-DD` |
| `--date-field` | 기간 기준 | `registered`(기본) `published` `surveyed` |
| `--gubun` | 선거구분 | `nesdc elections` 코드 (예: `VT044`) |

> 포털은 `--date-field` 없이는 기간 필터를 무시합니다 — kvote 는 기간 지정 시 자동으로 `registered` 를 기본 적용합니다.

## 게시판 (nesdc)

`kvote nesdc boards` 로 확인. 모두 동일한 엔진으로 처리됩니다.

| name | 내용 |
|---|---|
| `results` | 여론조사결과 보기 (상세 메타 + PDF) — 기본값 |
| `data` | 여론조사결과 주요 데이터 (주차별 벌크) |
| `notices` | 공지사항 |
| `library` | 자료실 |
| `actions` | 선거여론조사기관 조치현황 |
| `violations` | 유형별 위반사례 |

## 출력 형식

| `-f` | 용도 |
|---|---|
| `json` | 기본. 사람이 읽거나 `jq` 로 가공 |
| `jsonl` | 한 줄에 한 레코드. 대량 수집/스트리밍 |
| `table` | 터미널에서 바로 보기 (한글 폭 정렬) |

## 전역 플래그

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `-f, --format` | `json` | 출력 형식 |
| `--delay` | `700ms` | 요청 간 최소 간격 (rate limit) |
| `--base-url` | — | 포털 base URL 재정의 (테스트용) |

## 개발

```bash
make build    # 빌드
make test     # 테스트 (네트워크 불필요 — testdata 픽스처 사용)
make fmt      # gofmt
go vet ./...
```

자세한 내부 구조는 [CLAUDE.md](CLAUDE.md) 참고.

## 라이선스

MIT
