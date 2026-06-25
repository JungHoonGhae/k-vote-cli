# AGENTS.md — kvote를 쓰는 AI 에이전트를 위한 안내

이 문서는 **셸을 가진 AI 에이전트**(Claude Code 등)가 `kvote`로 한국 선거 공개 데이터를
**스스로 검증**할 수 있도록 돕는다. kvote는 MCP 서버가 아니라 **CLI**다 — 그냥 셸에서
호출하고 `jq`·`awk`·`sqlite3`로 합성하라. 터미널이 인터페이스다.

## 0. 먼저 알아야 할 계약 (Contract)

- **중립성**: kvote는 *어떤 분석적 입장도 취하지 않는다.* 원자료를 보존하고, 정의가 명확한
  표준 파생값(투표율·득표율·집계)만 제공한다. **무엇이 정상·비정상·수상한지 판단하지 않는다.**
  → 그 판단·해석·결론은 **너(에이전트)와 사용자의 몫**이다. kvote가 플래그·점수를 주지 않는다고
  해서 "문제 없음"이 아니다. 네가 원자료로 직접 검증해라.
- **키리스**: API 키 발급 불필요. 모두 공개 데이터.
- **예의**: 요청 간 기본 700ms rate limit(`--delay`로 조절). 대량 수집 시 그대로 둬라.
- **출력**: `-f json`(기본) / `jsonl`(대량·스트리밍, 한 줄 한 레코드) / `table`(사람용).
  **에이전트는 `-f jsonl` + `jq`를 기본으로 써라.** 큰 데이터(개표 CSV 7MB·18,000행)는
  컨텍스트에 통째로 넣지 말고 셸에서 필터해서 필요한 것만 읽어라.
- **데이터 출처**: `nesdc`=중앙선거여론조사심의위원회(여론조사), `nec`=중앙선거관리위원회 →
  data.go.kr 공개 파일(개표결과). `info.nec.go.kr`은 robots 차단이라 다루지 않는다.

## 1. 능력 지도 (Capability map)

| 명령 | 무엇을 주나 |
|---|---|
| **`kvote nec corpus [--normalize]`** | **핵심 개표결과(대선·총선·비례·지방 7·8회) 한 명령 동시 다운로드** (+`--normalize` 투표구별 JSONL). 키 불필요. **시작점** |
| `kvote nec results <pk> [--file F]` | 개표결과 정규화 (투표구별; `--aggregate` 집계, `--by-votetype` 분리, XLSX는 `--race --leaf-only`) |
| `kvote nec datasets [-q]` | 선관위 공개 파일 데이터 검색(개표결과 등) → `publicDataPk` |
| `kvote nec pull <pk>` | 개표결과 원본(CSV/XLSX) 다운로드 |
| `kvote nesdc sync [board]` | 여론조사 기간/조건 전체를 JSONL로 일괄 수집 (필터: `-q --field --from --to --date-field --gubun --pull`) |
| `kvote nesdc bulk` | data 게시판 누적 마스터 엑셀 → 정규화 정당지지율 레코드(2023.10.30~ 전체) |
| `kvote nesdc show <nttId> [--crosstab]` | 단건 상세 메타 + **표본 구성** 교차표(성별·연령·지역 × 완료·가중) |
| `kvote nesdc pull <nttId>` | 첨부 PDF(통계표·설문지) 다운로드 |
| `kvote nec latest <키워드>` | 선거종류 최신 회차 데이터셋 자동 해석 |
| `kvote nec elections [-q --sgtype]` | **선거코드 레지스트리** (모든 sgId·선거명·선거종류·투표일, 1987~) — OpenAPI |
| `kvote nec turnout <sgId> --sgtype N` | 투표율(시도/구시군별, 본·사전 분리) — OpenAPI |
| `kvote nec winners <sgId> --sgtype N` | 당선인(선거구·기호·정당·이름·득표수·득표율) — OpenAPI |
| `kvote api login / list / apply / config / logout` | data.go.kr 계정 연동(OpenAPI 활용신청·인증키·만료 자동 관리) |

OpenAPI 명령(`elections/turnout/winners`)은 인증키 필요 — `KVOTE_DATAGOKR_KEY` 환경변수 또는
`--api-key`. 키가 없으면 `kvote api login` → `kvote api apply <pk>` 로 발급(자동승인). 키는 비밀이니
로그·커밋 금지. `--sgtype`: 1=대통령 2=국회의원 3=시도지사 4=구시군장 5=시도의원 6=구시군의원.

전 명령은 `--help`로 자기 설명. 모르면 `kvote <cmd> --help`부터.

## 2. 검증 레시피 (Recipes)

> 모든 레시피의 패턴: **kvote가 중립 데이터를 jsonl로 → 네가 jq/계산으로 검증.**
> 결론은 네가 내린다.

### 2.1 사전투표 vs 본투표 득표율 비교 (개표 검증의 핵심 쟁점)

```bash
# 선거구 × 투표유형으로 집계 → 같은 선거구의 본투표/관내사전/관외사전 득표율을 나란히
kvote nec results 15025527 --aggregate sgg --by-votetype -f jsonl > /tmp/sgg.jsonl

# 예: 한 선거구에서 투표유형별 1위 후보 득표율(share)을 뽑아 비교 (네가 해석)
jq -r 'select(.district=="서울특별시 종로구")
       | "\(.voteType)\t\(.candidates[0].party) \(.candidates[0].name)\t\(.candidates[0].share)"' /tmp/sgg.jsonl
```
kvote는 사전·본을 *분리해서 제공만* 한다. "격차가 크다/이상하다"는 판단은 네가 데이터로.

### 2.2 개표 항등식 직접 확인 (투표수 = 후보합 + 무효)

```bash
# 투표구 원단위(leaf)로 받아, 각 단위의 항등식이 맞는지 네가 계산
kvote nec results 15025527 -f jsonl \
  | jq -c 'select(.aggregate != true)
           | {unit:[.sido,.district,.town,.booth], votes,
              sum:( ([.candidates[].votes]|add) + .invalid ),
              ok:( .votes == ([.candidates[].votes]|add) + .invalid )}' \
  | jq -c 'select(.ok==false)'   # 안 맞는 단위만 (있으면 너가 원인 조사)
```
kvote는 선거인수·투표수·무효·후보별 득표를 **빠짐없이** 준다. 항등식 검산은 네 몫.

### 2.3 표본 대표성·가중 확인 (여론조사)

```bash
kvote nesdc show 19366 --crosstab -f json \
  | jq '{total, weighting, marginError,
         "성별": (.crosstabs[] | select(.dimension=="성별") | .cells)}'   # jq 객체 키가 한글이면 반드시 따옴표
# 완료 사례수 vs 가중 사례수 차이를 보고 가중 강도를 네가 평가
```

### 2.4 여론조사 전수 수집 → 기관×방식 분포 (대규모 패턴)

```bash
kvote nesdc sync --from 2026-01-01 -f jsonl > /tmp/polls.jsonl
# 집계는 jq-네이티브로. (셸 `sort | uniq -c`는 일부 환경에서 한글을 잘못 묶으니 피하라.)
jq -rs 'group_by(.values["조사기관명"] + "|" + .values["조사방법"])
        | map({k:(.[0].values["조사기관명"] + " / " + .[0].values["조사방법"]), n:length})
        | sort_by(-.n)[] | "\(.n)\t\(.k)"' /tmp/polls.jsonl
```

### 2.5 정당지지율 누적 시계열 (단일 파일)

```bash
kvote nesdc bulk -f jsonl > /tmp/party.jsonl   # 2023.10.30~ 전체 정당지지율 1400+건
jq -c 'select(.agency=="한국갤럽조사연구소") | {surveyDate, partySupport}' /tmp/party.jsonl
```

### 2.6 데이터셋 발견 → 정규화 (선관위 개표결과 전반)

```bash
kvote nec datasets -q 개표결과 -f table       # publicDataPk 고른다
kvote nec results <publicDataPk> -f jsonl     # CSV/XLSX 자동 판별, 같은 스키마
# 지방선거(XLSX 멀티시트)는 선거종류 지정 + 집계행 제외:
kvote nec results 15101509 --file ./downloads/*.xlsx --race 교육감 --leaf-only -f jsonl
```

### 2.7 OpenAPI 대규모 전수 수집 (분석가용)

`nec elections`로 모든 선거를 **열거**한 뒤 `turnout`/`winners`를 **루프로 전수** 받는다.
출력이 JSONL이라 그대로 duckdb/pandas에 흘려보낼 수 있다.

```bash
export KVOTE_DATAGOKR_KEY=...   # api login → api apply 로 발급

# 모든 대선의 시도별 투표율을 한 파일로
kvote nec elections -q 대통령선거 --sgtype 1 -f jsonl \
  | jq -r .sgId \
  | while read id; do kvote nec turnout "$id" --sgtype 1 -f jsonl; done > /tmp/pres_turnout.jsonl

# 모든 총선 당선인(역대 지역구)을 한 파일로
kvote nec elections -q 국회의원선거 --sgtype 2 -f jsonl \
  | jq -r .sgId \
  | while read id; do kvote nec winners "$id" --sgtype 2 -f jsonl; done > /tmp/winners.jsonl

# duckdb 로 교차 질의 (예: 정당별 당선 수 시계열)
duckdb -c "SELECT sgId, party, count(*) n FROM read_json_auto('/tmp/winners.jsonl')
           GROUP BY 1,2 ORDER BY 1,3 DESC"
```
정중한 수집: `--delay`(기본 700ms)가 요청 간격을 보장한다. 전수 루프도 이 간격을 지킨다.

## 3. 큰 데이터 다루는 법 (Context 절약)

- **절대** 7MB CSV·수만 행을 통째로 읽어 컨텍스트에 넣지 마라.
- `kvote … -f jsonl | jq 'select(…)'` 로 **셸에서 먼저 줄여라.**
- 반복 교차 질의가 필요하면 sqlite/duckdb에 적재:
  ```bash
  kvote nec results 15025527 -f jsonl > /tmp/r.jsonl
  duckdb -c "SELECT district, voteType, sum(votes) FROM read_json_auto('/tmp/r.jsonl')
             WHERE aggregate=false GROUP BY 1,2"
  ```

## 4. 스키마 요점 (파싱 신뢰용)

- **nec results (CSV/공통)**: `{sido,district,town,booth,voteType,electorate,votes,invalid,abstention,candidates:[{party,name,votes}]}`.
  집계 뷰는 `{level,sido,district,town,voteType,electorate,votes,invalid,validVotes,abstention,turnout,candidates:[{party,name,votes,share}]}`.
- **nec results (XLSX 공통)**: `{race,dimensions:[{label,value}],voteType,aggregate,electorate,votes,invalid,abstention,candidates:[…]}`.
  차원은 고정 필드가 아니라 **라벨 보존**(선거종류마다 다름). `aggregate==false`가 중복 없는 분할.
- **voteType**: `본투표`/`관내사전`/`관외사전`/`거소`. **득표율 정의**: `share = 후보득표 / 유효투표수`,
  `turnout = 투표수 / 선거인수`, `유효투표수 = 투표수 − 무효`. 다른 정의가 필요하면 원자료로 직접 계산.
- **nesdc --crosstab**: `{total,crosstabs:[{dimension,cells:[{category,completed,weighted}]}],weighting,marginError}`.
- **nec turnout** (OpenAPI): `{sgId,sgTypecode,sido,gusigun,electorate,votes,turnout,psSunsu,psEtcSunsu,psTusu,psEtcTusu}`
  — `sido/gusigun="합계"`는 집계 행. `ps*/psEtc*`는 API의 본투표 명부/사전 성격 2분할(verbatim 보존).
- **nec winners** (OpenAPI): `{sgId,sgTypecode,sido,sgg,gusigun,giho,party,name,...,votes,voteRate}` — `votes/voteRate`=득표수/득표율.
- **nec elections** (OpenAPI): `{sgId,sgName,sgTypecode,voteDate}` — `sgTypecode 0`=상위 항목, `1~7`=개별 선거(turnout/winners용).

## 5. 하지 말 것

- kvote에게 "이 선거 조작됐어?" 같은 판단을 기대하지 마라 — 그건 데이터를 주지, 답을 주지 않는다.
- 집계행(`aggregate==true`, 합계/소계)을 leaf와 섞어 합산하지 마라(중복). `--leaf-only` 또는
  `select(.aggregate!=true)`.
- 표본 구성(완료 vs 가중)을 득표 결과와 혼동하지 마라 — 전자는 "누구를 조사했나", 후자는 PDF/개표.
- **한글 값 집계에 셸 `sort | uniq -c`를 쓰지 마라** — 환경에 따라 다중바이트를 잘못 묶어 *조용히
  틀린 수*를 낸다(실측됨). 분포·집계는 `jq -rs 'group_by(...)'` 같은 jq-네이티브로 해라.
- jq 객체 리터럴의 **한글 키는 따옴표 필수**: `{"성별": …}` (따옴표 없으면 구문 오류).
