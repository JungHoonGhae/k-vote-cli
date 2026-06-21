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

## 3. 공통 스키마

기존 P1 `ResultRecord`(CSV 전용, 평면 4차원)는 race·구분을 담지 못한다. **선거종류 인지
공통 레코드**를 신설하고, CSV·XLSX 모두 이쪽으로 수렴시킨다.

```go
// internal/nec/election.go
type ElectionResult struct {
	Race       string `json:"race"`               // 선거종류 (CSV는 데이터셋 제목 유래, XLSX는 시트명)
	Sido       string `json:"sido"`
	Gusigun    string `json:"gusigun,omitempty"`   // 구시군명
	District   string `json:"district,omitempty"`  // 선거구명 (없을 수 있음: 비례·일부)
	Town       string `json:"town,omitempty"`      // 읍면동명
	Gubun      string `json:"gubun,omitempty"`      // 원본 구분 verbatim (합계/소계/거소투표/관외사전투표/관내사전투표/투표구)
	VoteType   string `json:"voteType"`            // 파생: 본투표/관내사전/관외사전/거소  (집계행은 "")
	Aggregate  bool   `json:"aggregate"`           // 파생: 합계/소계 등 집계행이면 true (leaf면 false)
	Electorate int    `json:"electorate"`
	Votes      int    `json:"votes"`
	Invalid    int    `json:"invalid"`
	Abstention int    `json:"abstention"`
	Candidates []CandidateVote `json:"candidates"` // 기존 타입 재사용 (비례=Name "", 교육감=Party "")
}
```

- **원자료 보존**: 구분을 verbatim 으로 둔다. `voteType`·`aggregate` 는 정의가 명시된 중립 파생.
- **집계행 처리**: 버리지 않는다. `aggregate=true` 로 태깅해 보존 → 소비자가 leaf만 쓰려면
  `aggregate==false` 필터, 소스 자체 소계를 쓰려면 그대로. (중립 — 우리가 고르지 않는다.)
- `Candidates` 는 P1 의 `CandidateVote{Party,Name,Votes}` 재사용. 비례는 Name 빈칸, 교육감은
  Party 빈칸.

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
- `results.go` (수정): `ParseResults`(CSV) 가 `[]ElectionResult` 도 낼 수 있도록 — 기존
  `ResultRecord` 는 유지하되 `ToElectionResults()` 어댑터를 더하거나, CSV 파서가 직접
  `ElectionResult` 를 내도록 전환. **하위호환**: 기존 `nec results` 의 `ResultRecord` 출력과
  P1 집계(`Aggregate`)는 그대로 동작해야 한다 → 어댑터 방식 권장(작은 변환 함수).
- `aggregate.go` (P1, 영향 최소): 집계는 `ResultRecord` 위에서 동작. XLSX→집계가 필요하면
  `ElectionResult`→`ResultRecord` 매핑 어댑터로 재사용하거나, P3 로 미룬다(아래 비범위).

### 4.1 헤더 기반 열 감지 (시트별 가변 대응)

row0 라벨로 인덱스를 찾는다:
- 차원 열: 처음부터 `선거인수`(첫 등장) 전까지 — 라벨별로 Sido/Gusigun/District/Town/구분 매핑.
  (`시도명`|`선거구명`→ 첫 열은 시도, `구시군명`, `선거구(...)`|`선거구명`→District, `읍면동명`, `구분`.)
- 후보 블록: `후보자별 득표수` 가 걸친 열 범위 = row0 의 그 라벨부터 `계` 전까지.
- 꼬리: `계`·`무효투표수`·`기권수`.
- 후보 정의: 그 선거구의 최근 후보 정의행(읍면동·구분 모두 빈 행)의 후보 열 텍스트.

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
- `TestParseResultsXLSX`: 시트별 race, 가변 차원 매핑, wide→long, 후보 재정의 추적, 비례(Name 빈칸).
- `TestVoteTypeAggregateDerivation`: 구분→voteType/aggregate 규칙(특히 거소/관외사전의 aggregate=true).
- `TestXLSXLeafOnlyFilter`: leaf 합이 합계행과 일치(소스 정합성 — 우리가 판단하는 게 아니라
  파서 정확성 검증).
- `TestDetectXLSXvsCSV`: 매직바이트 판별.

## 8. 비범위 (이번 P2)

- XLSX→P1 `Aggregate` 연동(시도/전국 롤업)은 **P2 범위 밖**. P2 는 정규화·보존까지. 집계는
  CSV 와 XLSX 가 공통 `ElectionResult` 로 모인 뒤 별도 phase 에서 통합(또는 후속).
- 투표율/후보자/당선인 등 다른 데이터셋(P3) 무관.
- 어떤 "이상치"·격차·검증 판단도 없음(중립성).
