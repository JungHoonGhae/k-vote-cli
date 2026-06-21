---
name: kvote-verify
description: >-
  kvote CLI로 한국 선거 공개 데이터(NEC 개표결과·NESDC 여론조사)를 받아 중립적으로
  검증·분석하는 워크플로. 사용자가 개표결과 검증, 사전투표 vs 본투표 득표율 비교,
  개표 수치 항등식 검산, 여론조사 표본 대표성·가중 확인, 특정 선거/기관 데이터 수집,
  "이번 지방선거/총선/대선 데이터 좀 봐줘" 류를 요청할 때 반드시 사용. kvote·개표·
  사전투표·득표율·투표율·여론조사·표본·data.go.kr·선관위 데이터가 언급되면 트리거.
  핵심: kvote는 데이터만 제공하고 우리는 판단하지 않는다(중립). 또한 한글 데이터에서
  jq/셸을 잘못 쓰면 조용히 틀리므로 이 skill의 관용구를 따른다.
---

# kvote로 선거 데이터 중립 검증하기

`kvote`는 한국 선거 공개 데이터를 키 없이 받아 구조화(JSON/JSONL)하는 CLI다. 이 skill은
그걸로 **의미 있는 검증을 실제로 돌리는** 법을 담는다. 빌드는 `make build`(→ `./bin/kvote`).

## 0. 중립성 — 가장 중요 (어기면 이 도구의 존재 이유가 무너진다)

kvote도, 너도 **어떤 분석적 입장도 취하지 않는다.** 데이터를 정확히 surface 하고, 정의가
명확한 표준 파생값(득표율·투표율·집계)만 계산한다. **무엇이 정상·비정상·조작·수상한지
단정하지 마라.** 결과를 제시할 때:

- 수치는 중립적으로 기술한다. "A의 사전 득표율이 본투표보다 8%p 높다"(O) /
  "이건 조작 증거다"(X) / "이건 완전히 정상이다"(X).
- 알려진 맥락이 있으면 *양쪽 다* 짧게 언급하되 어느 쪽도 편들지 않는다. 예: 사전-본 격차는
  한국 모든 선거에서 반복되는 현상이고, 인구통계학적 자기선택 설명과 의혹 제기가 둘 다 있다.
- "해석은 사용자 몫"임을 명시한다. 우리의 가치는 **누구나 같은 명령으로 재현**하게 하는 것.

## 1. 워크플로 (발견 → 수집 → 정규화 → 검증)

```bash
# (1) 데이터셋 발견 — 선관위가 공개한 개표결과 파일
kvote nec datasets -q 개표결과 -f jsonl | jq -r '"\(.publicDataPk)\t\(.formats|join(","))\t\(.title)"'

# (2) 정규화 — CSV(총선·대선)/XLSX(지방선거) 자동 판별, 같은 스키마
kvote nec results <publicDataPk> -f jsonl > /tmp/r.jsonl        # 다운로드+정규화
kvote nec results --file ./already.csv -f jsonl > /tmp/r.jsonl  # 이미 받은 파일

# (3) 검증 — 아래 §3 레시피
```

데이터가 아직 data.go.kr에 없으면(예: 막 끝난 선거) 솔직히 그렇게 말하고, 같은 종류의
직전 선거로 방법론을 실증해라(`pk`만 바꾸면 동일하게 돌아간다).

## 2. 한글 데이터 함정 — 반드시 지켜라 (안 지키면 조용히 틀린 수가 나온다)

이건 실측으로 깨진 것들이다. 추측 말고 이대로 해라:

- **셸 `sort | uniq -c`를 한글 값 집계에 쓰지 마라.** 환경에 따라 다중바이트를 잘못 묶어
  *틀린 수*를 낸다(예: 18,826건을 전부 한 범주로 뭉갬). 집계는 **jq-네이티브**로:
  ```bash
  jq -rs 'group_by(.voteType)[] | "\(length)\t\(.[0].voteType)"' /tmp/r.jsonl
  ```
- **jq 객체 리터럴의 한글 키는 따옴표 필수.** `{성별: …}`는 구문 오류 → `{"성별": …}`.
- **큰 데이터는 셸에서 먼저 줄여라.** 7MB·수만 행을 컨텍스트에 통째로 넣지 말고
  `kvote … -f jsonl | jq 'select(…)'`로 필요한 것만. 반복 교차질의는 duckdb 적재:
  ```bash
  duckdb -c "SELECT district,voteType,sum(votes) FROM read_json_auto('/tmp/r.jsonl')
             WHERE aggregate=false GROUP BY 1,2"
  ```

## 3. 검증 레시피

### 3.1 사전투표 vs 본투표 득표율 (개표 검증의 핵심 쟁점)

투표유형은 `본투표 / 관내사전 / 관외사전 / 거소(선상)`. **사전 = 관내사전+관외사전+거소,
본 = 본투표.** XLSX(지방선거)는 집계행이 섞여 있으니 **반드시 `--leaf-only`**로 leaf만
받아라(`aggregate==true`인 합계/소계를 빼야 중복 합산이 안 된다).

```bash
kvote nec results --file <xlsx> --race 시·도지사 --leaf-only -f jsonl > /tmp/leaf.jsonl

# 시도별 양대 후보의 본 vs 사전 득표율 + 격차 (영문 키 주의)
jq -rs '
  def csum($r): reduce ($r[].candidates[]) as $c ({}; .[$c.name] += $c.votes);
  group_by(.dimensions[0].value)[]
  | .[0].dimensions[0].value as $sido
  | (map(select(.voteType=="본투표"))) as $bon
  | (map(select(.voteType!="본투표"))) as $pre
  | csum($bon) as $b | csum($pre) as $p
  | ([$b[]]|add) as $bt | ([$p[]]|add) as $pt
  | ($b|to_entries|sort_by(-.value)|.[0:2][])
  | .key as $n
  | "\($sido)\t\($n)\t본 \(($b[$n]/$bt*100*10|round/10))%\t사전 \(($p[$n]/$pt*100*10|round/10))%\t사전-본 \((($p[$n]/$pt-$b[$n]/$bt)*100*10|round/10))%p"
' /tmp/leaf.jsonl
```

### 3.2 개표 항등식 검산 (투표수 = 후보합 + 무효)

kvote는 원자료를 빠짐없이 준다. 검산은 네가 한다. CSV(총선·대선)는 전부 leaf:

```bash
kvote nec results --file <csv> -f jsonl \
  | jq -c 'select(.aggregate!=true)
           | {unit:[.sido,.district,.town,.booth],
              ok:(.votes == ([.candidates[].votes]|add)+.invalid)}' \
  | jq -c 'select(.ok==false)'    # 안 맞는 단위만 (있으면 원인은 네가 조사)
```

XLSX는 정합성을 leaf합=합계로 확인할 수 있다: 한 선거구의 `aggregate==false` 후보합이
그 선거구 `합계` 행(`aggregate==true`)과 같아야 한다.

### 3.3 여론조사 표본 대표성·가중 확인

```bash
kvote nesdc show <nttId> --crosstab -f json \
  | jq '{total, weighting, marginError,
         "성별": (.crosstabs[] | select(.dimension=="성별") | .cells)}'
# 완료 사례수 vs 가중 사례수 차이로 가중 강도를 보되, "과대가중"이라 단정하지 마라.
```

### 3.4 여론조사 전수 수집 → 기관×방식 분포

```bash
kvote nesdc sync --from 2026-01-01 -f jsonl > /tmp/polls.jsonl
jq -rs 'group_by(.values["조사기관명"]+"|"+.values["조사방법"])
        | map({k:(.[0].values["조사기관명"]+" / "+.[0].values["조사방법"]), n:length})
        | sort_by(-.n)[] | "\(.n)\t\(.k)"' /tmp/polls.jsonl
```

## 4. 스키마 요점 (파싱 신뢰용)

- **nec results (CSV/공통 leaf)**: `{sido,district,town,booth,voteType,electorate,votes,invalid,abstention,candidates:[{party,name,votes}]}`.
- **nec results (XLSX 공통)**: `{race,dimensions:[{label,value}],voteType,aggregate,electorate,votes,invalid,abstention,candidates:[…]}`.
  차원은 고정 필드가 아니라 **라벨 보존**(선거종류마다 다름). `dimensions[0].value`가 보통 시도/선거구.
  `aggregate==false`가 중복 없는 분할(본+관내사전+관외사전+거소 = 합계).
- **정의**: `투표율=투표수/선거인수`, `유효투표수=투표수−무효`, `후보 득표율=후보득표/유효투표수`.
  다른 정의가 필요하면 원자료로 직접 계산.
- **nesdc --crosstab**: `{total,crosstabs:[{dimension,cells:[{category,completed,weighted}]}],weighting,marginError}`
  — 이건 *표본 구성*(누구를 조사했나)이지 득표가 아니다. 이념·연령별 지지율은 첨부 PDF 전용.

전체 레시피·계약은 저장소 루트 `AGENTS.md` 참고.

## 5. 결과 보고 형식

표/요약으로 수치를 제시하고, 끝에 한 줄로 중립 입장과 재현 명령을 남겨라:

> 위 수치는 `kvote nec results <pk> --leaf-only`로 누구나 재현할 수 있습니다. kvote는
> 데이터를 surface 할 뿐 정상/이상을 판단하지 않으며, 해석은 사용자의 몫입니다.
