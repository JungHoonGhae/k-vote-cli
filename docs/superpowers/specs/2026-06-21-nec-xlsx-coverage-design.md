# Phase 2: NEC 개표 완전 커버리지 (XLSX 통합) 설계

작성일: 2026-06-21
상태: 설계 (리뷰 대기)
상위: `2026-06-21-kvote-coverage-roadmap-design.md` 의 P2

## 1. 목표

선관위가 data.go.kr 에 올린 **XLSX 형식 개표결과**(지방선거·재보궐 등)를, CSV(총선·대선)와
**하나의 공통 스키마**로 수렴해 정규화한다. 이로써 모든 선거종류를 같은 도구·같은 출력 형태로
소비자(사람/AI 에이전트)가 받을 수 있게 한다.

중립성 원칙(상위 spec §1.1) 그대로: 원자료 완전 보존 + 정의 명시 표준 파생값만. 집계행을
버리거나 "이상" 판단 없이, 구분을 그대로 보존하고 소비자가 필터한다.

## 2. 정찰 결론 (제8회 지방선거 XLSX 실측)

- **멀티시트 = 선거종류**: 시·도지사 / 구·시·군의장 / 시·도의회의원 / 구·시·군의회의원 /
  광역의원비례대표 / 기초의원비례대표 / 교육감 / 교육의원 (8개).
- **Wide-format**: 후보가 *열*. CSV(후보가 행)와 반대.
- **선행 차원 열이 시트마다 다름**(4~6개): 시도·구시군·선거구·읍면동·구분의 조합. 선거구는
  시도지사/교육감(=시도)·구시군의장(=구시군)·시도의원/구시군의원(=지역구명)에 따라 의미가
  다르고, 비례대표엔 선거구 열이 없다.
- **3행 헤더**: row0=라벨, row1=병합셀 잔재(무시), row2=후보 정의(`정당\n이름`; 비례=정당만,
  교육감=이름만). 후보 정의행은 **선거구가 바뀔 때마다 다시 나오고 후보 수도 가변**.
- **구분 열** 값: `합계`(선거구 총계) · `소계`(읍면동 총계) · `거소투표` · `관외사전투표` ·
  `관내사전투표` · (읍면동의 leaf 투표구). 즉 집계행과 leaf행이 한 시트에 섞여 있다.
- **꼬리 열**: `계`(유효합) · `무효투표수` · `기권수`.

## 3. 공통 스키마 (라벨 기반 일반화 — 선거별 손매핑 없음)

**설계 철학**: 선거종류·연도마다 *선행 차원 열은 다르지만 지표 앵커는 일관*하다
(`선거인수`·`투표수`·`후보자별 득표수`·`계`·`무효투표수`·`기권수`). 그래서 차원을 고정 필드로
욱여넣지 않고 — 그러면 선거마다 매핑이 늘어난다 — **헤더 라벨을 키로 일반 캡처**한다. 매핑하는
것은 안정적인 앵커 라벨뿐이며, 그것이 유지되는 한 새 선거종류·시트는 코드 변경 없이 통과한다.
기존 P1 `ResultRecord`(CSV 전용, 평면 4차원)도 이 일반 모델의 특수 케이스다.

```go
// internal/nec/election.go

// Dimension is one source column to the left of the 선거인수 anchor, kept under
// its verbatim header label. The set/order varies by election type and year;
// we capture whatever is there rather than mapping each layout by hand.
type Dimension struct {
	Label string `json:"label"` // row0 헤더 라벨 (시도명/구시군명/선거구명/읍면동명/구분 …)
	Value string `json:"value"`
}

type ElectionResult struct {
	Race       string      `json:"race"`        // 선거종류 (XLSX=시트명, CSV=데이터셋 제목 유래)
	Dimensions []Dimension `json:"dimensions"`  // 앵커(선거인수) 앞 모든 열, 라벨 보존·순서 유지
	VoteType   string      `json:"voteType"`    // 파생: 본투표/관내사전/관외사전/거소 (집계행은 "")
	Aggregate  bool        `json:"aggregate"`   // 파생: 합계/소계 등 집계행이면 true (leaf면 false)
	Electorate int         `json:"electorate"`
	Votes      int         `json:"votes"`
	Invalid    int         `json:"invalid"`
	Abstention int         `json:"abstention"`
	Candidates []CandidateVote `json:"candidates"` // 기존 타입 재사용 (비례=Name "", 교육감=Party "")
}
```

- **원자료 완전 보존**: 모든 차원을 라벨과 함께 verbatim 으로 — 구분 포함. 우리가 차원을
  해석·정규화하지 않으므로 어떤 선거 형태가 와도 손실/오매핑이 없다.
- **매핑 대상은 앵커뿐**: 지표 4종·후보 블록을 라벨로 감지(§4.1). 차원은 일반 캡처. → 선거마다
  개별 매핑이 **없다**. 새 포맷이 앵커 라벨까지 바꾸면 그 시트는 skip+경고(§6) — 추측 매핑 안 함.
- **편의 접근자**(코드용, 스키마 아님): `(ElectionResult).Dim(label) string` 으로 "시도명" 등을
  라벨로 조회. JSON 소비자(AI 에이전트)는 `dimensions` 를 그대로 질의.
- **집계행 처리**: 버리지 않는다. `aggregate=true` 로 태깅 보존 → 소비자가 leaf만 쓰려면
  `aggregate==false` 필터, 소스 소계를 쓰려면 그대로. (중립 — 우리가 고르지 않는다.)
- `Candidates` 는 P1 의 `CandidateVote{Party,Name,Votes}` 재사용. 비례는 Name 빈칸, 교육감은
  Party 빈칸.

### 3.0 voteType/aggregate 가 참조하는 차원

파생은 "구분" 라벨 차원의 값과 "읍면동명"(있으면) 차원의 공백 여부로 판정한다(§3.1). CSV(P1)는
"구분" 열이 없어 booth/town 으로 판정 — 어댑터에서 동일한 `VoteType`/`Aggregate` 를 채운다.

### 3.1 voteType / aggregate 파생 규칙 (고정)

구분(공백 제거 후) 기준:
- `거소투표` → voteType=`거소`, aggregate=false … (단, 읍면동 비어있는 선거구-level 거소/관외는
  aggregate=true 로 본다 — 아래 판정)
- `관외사전투표` → voteType=`관외사전`
- `관내사전투표` → voteType=`관내사전`
- `합계` → aggregate=true, voteType=""
- `소계` → aggregate=true, voteType=""
- 그 외(투표구명/읍면동 leaf) → voteType=`본투표`, aggregate=false

**aggregate 판정의 핵심**: 읍면동명이 비어 있고 구분이 합계/거소투표/관외사전투표인 행은
**선거구 단위 집계**(aggregate=true). 읍면동명이 있고 구분이 소계인 행은 **읍면동 집계**
(aggregate=true). 읍면동명이 있고 구분이 관내사전투표 또는 투표구명인 행이 **leaf**
(aggregate=false). → 즉 `aggregate = (구분 in {합계,소계}) || (읍면동 비어있음 && 구분 in {거소투표,관외사전투표})`.
거소/관외사전은 본래 선거구 단위로만 집계되므로 leaf 가 아니다.

## 4. 컴포넌트 (internal/nec)

- `election.go` (신규): `ElectionResult` 타입 + voteType/aggregate 파생 헬퍼.
- `xlsx.go` (신규): `ParseResultsXLSX(raw []byte) ([]ElectionResult, error)` — excelize 로 전 시트
  순회, 헤더 라벨로 열 경계 감지, 후보 정의행 추적, wide→long 변환.
- `results.go` (수정): 기존 `ResultRecord`(CSV)와 P1 집계(`Aggregate`)는 **그대로 유지**(하위호환).
  CSV→공통모델은 `ResultRecord.ToElectionResult()` 어댑터로 제공 — 차원을 라벨 4종
  (시도명/선거구명/법정읍면동명/투표구명)으로 `Dimensions` 에 담고, 기존 VoteType/필드를 옮긴다.
  이로써 CSV·XLSX 가 같은 `ElectionResult` 로 모인다.
- `aggregate.go` (P1, 영향 최소): 집계는 `ResultRecord` 위에서 동작. XLSX→집계가 필요하면
  `ElectionResult`→`ResultRecord` 매핑 어댑터로 재사용하거나, P3 로 미룬다(아래 비범위).

### 4.1 헤더 기반 열 감지 (시트별 가변 대응 — 손매핑 없음)

row0 라벨로 인덱스를 찾는다. **차원 열은 라벨별 고정 매핑을 하지 않고 그대로 캡처**한다:
- 차원 열: 0번부터 `선거인수`(첫 등장) 전까지 — 각 열을 `Dimension{Label: row0[i], Value: …}`
  로 순서대로 담는다. 라벨이 무엇이든(시도명/선거구명/구시군명/선거구(...)/읍면동명/구분) 보존만.
- 지표 앵커: `선거인수`(첫 등장)=Electorate, 다음 `투표수`=Votes, 꼬리의 `무효투표수`=Invalid,
  `기권수`=Abstention. 라벨로 찾으므로 열 위치 변화에 무관.
- 후보 블록: `후보자별 득표수` 가 걸친 열 범위 = 그 라벨 열부터 `계` 전까지.
- 후보 정의: 그 선거구의 최근 후보 정의행(차원 중 읍면동·구분이 비고 후보 열에 `정당\n이름`이
  있는 행)의 후보 열 텍스트. 선거구가 바뀌면 새 정의행으로 갱신.
- **불변 가정은 앵커 라벨뿐**: `선거인수`/`투표수`/`후보자별 득표수`/`계`/`무효투표수`/`기권수`.
  이게 있으면 차원이 어떻게 바뀌어도 파싱된다. 없으면 그 시트 skip+경고(§6).

## 5. CLI

`nec results` 는 CSV·XLSX 자동 판별(파일 시그니처: XLSX 는 `PK\x03\x04` zip 매직).
- XLSX 면 `ParseResultsXLSX` → `ElectionResult` 출력.
- `--race <시트명>` 옵션: 특정 선거종류만(부분일치). 기본 전체.
- `--leaf-only` 옵션: `aggregate==false` 행만(중복합산 방지용 편의 필터 — 데이터 변형 아님, 단순 필터).
- 기존 CSV 경로: `ResultRecord` 출력 유지(하위호환). *옵션* `--schema election` 으로 CSV 도
  `ElectionResult` 로 받게 할지는 구현 시 결정(YAGNI — 우선 XLSX 만 ElectionResult).

## 6. 에러 처리

- 시트의 헤더에서 `선거인수`/`후보자별 득표수` 를 못 찾으면 그 시트는 건너뛰고 stderr 경고
  (포맷이 다른 신규 시트 대비). 전 시트 실패 시 에러.
- 후보 열 수와 후보 정의행 불일치: 가능한 만큼 매핑하고 남는 빈 열은 무시.
- 천단위 콤마 제거(P1 `atoiLoose` 재사용).

## 7. 테스트

- `internal/nec/testdata/` 에 **작은 XLSX 픽스처**를 코드로 생성(excelize)하거나 커밋. 2개 시트
  (후보형 1 + 비례형 1), 선거구 2개(후보 재정의), 구분 혼재(합계/소계/거소/관외사전/관내사전/투표구).
- `TestParseResultsXLSX`: 시트별 race, **가변 차원 일반 캡처**(4열 시트 vs 5열 시트가 둘 다
  라벨대로 Dimensions 에 들어감), wide→long, 후보 재정의 추적, 비례(Name 빈칸).
- `TestParseSkipsUnanchoredSheet`: 앵커(선거인수/후보자별 득표수) 없는 시트는 skip+경고, 다른
  시트는 정상 — 손매핑 대신 안전 실패 검증.
- `TestVoteTypeAggregateDerivation`: 구분→voteType/aggregate 규칙(특히 거소/관외사전의 aggregate=true).
- `TestXLSXLeafOnlyFilter`: leaf 합이 합계행과 일치(소스 정합성 — 우리가 판단하는 게 아니라
  파서 정확성 검증).
- `TestDetectXLSXvsCSV`: 매직바이트 판별.

## 8. 비범위 (이번 P2)

- XLSX→P1 `Aggregate` 연동(시도/전국 롤업)은 **P2 범위 밖**. P2 는 정규화·보존까지. 집계는
  CSV 와 XLSX 가 공통 `ElectionResult` 로 모인 뒤 별도 phase 에서 통합(또는 후속).
- 투표율/후보자/당선인 등 다른 데이터셋(P3) 무관.
- 어떤 "이상치"·격차·검증 판단도 없음(중립성).
