# Phase 4: NESDC 표본 구성 교차표 구조화 설계

작성일: 2026-06-21
상태: 설계 (리뷰 대기)
상위: `2026-06-21-kvote-coverage-roadmap-design.md` 의 P4

## 1. 목표

NESDC 상세(`view.do`)의 `Detail.Fields`(176행 lossless)에 들어있는 **표본 구성 교차표** —
성별·연령대별·지역별 × (조사완료 사례수 / 가중값 적용 사례수) — 를 깔끔한 구조로 노출한다.
"누구를 표본으로 뽑고 어떻게 가중했나"는 여론조사 대표성·가중 투명성의 핵심 검증 축이며, 이미
수집된 데이터를 구조화만 하면 된다(새 수집 없음).

**범위 명확화(정찰 결론):** 이념별/연령별 *정당지지율*(vote-by-demographic)은 상세 HTML 에
없고 **첨부 PDF 전용**이며 기관마다 레이아웃이 달라 별도의 큰 작업이다 → **후속 Phase**로 분리.
P4 는 상세 HTML 의 표본 구성만 다룬다.

중립성(상위 §1.1): 원자료(완료·가중 사례수) 그대로 보존, 구조화만. 대표성·과대가중 판단은 하지
않는다 — 소비자(사람/AI 에이전트)가 본다.

## 2. 정찰 결론 (nttId 19366 실측)

`Detail.Fields` 의 표본 구성 블록은 다음 행 시퀀스다:

```
[13] L=['구분','조사완료 사례수(명)','가중값 적용 기준 사례수(명)']   ← 헤더
[14] L=['전체']                 V=['1001','1001']
[15] L=['성별','남']            V=['546','496']     ← 2라벨 = 차원 시작
[16] L=['여']                  V=['455','505']     ← 1라벨 = 연속
[17] L=['연령대별','18~29세']    V=['128','149']     ← 새 차원
[18..22] 30대/40대/50대/60대/70세 이상
[23] L=['지역별','서울']        V=['198','185']     ← 새 차원
[24..30] 인천/경기 … 제주
```

규칙: **labels 가 2개면 `[차원, 범주]`로 차원 시작**, **1개면 `[범주]`로 직전 차원에 연속**.
값은 `[완료 사례수, 가중 사례수]`. `전체` 행은 차원 없는 합계. 헤더 다음부터 시작해, 값이
숫자쌍이 아닌 행(예: `조사방법1`, `피조사자 선정방법`)을 만나면 블록 종료.

가중방법은 `[165] L=['기본가중','산출방법'] V=['성별·연령별·지역별 가중값 부여 …']`,
표본오차는 `[169] L=['표본오차'] V=['95% 신뢰수준에 ±3.1%P']`.

## 3. 스키마

```go
// internal/nesdc/composition.go
type CompositionCell struct {
	Category  string `json:"category"`  // 남 / 18~29세 / 서울
	Completed int    `json:"completed"` // 조사완료 사례수
	Weighted  int    `json:"weighted"`  // 가중값 적용 기준 사례수
}

type Crosstab struct {
	Dimension string            `json:"dimension"` // 성별 / 연령대별 / 지역별
	Cells     []CompositionCell `json:"cells"`
}

type SampleComposition struct {
	Total       *CompositionCell `json:"total,omitempty"` // 전체 행
	Crosstabs   []Crosstab       `json:"crosstabs"`
	Weighting   string           `json:"weighting,omitempty"`   // 가중 산출방법
	MarginError string           `json:"marginError,omitempty"` // 표본오차
}
```

- 원자료 보존: 완료·가중 사례수를 그대로. 비율·격차 등 파생/판단 없음.
- `Total` 은 전체 합계(차원 없음). 차원별 Cells 합이 Total 과 같은지 등은 소비자가 검증.

## 4. 컴포넌트 (internal/nesdc)

- `composition.go` (신규): `func SampleCompositionOf(d *Detail) *SampleComposition` — 이미 파싱된
  `Detail.Fields` 위에서 동작(네트워크 무관, 순수). 블록 못 찾으면 nil.
- `detail.go` (수정 최소): `Detail` 구조는 그대로. 교차표는 파생이므로 별도 함수로 두고 `Detail`
  에 필드 추가는 하지 않는다(lossless 원칙 유지, 중복 저장 회피).

### 4.1 파싱 알고리즘 (Fields 기반)

1. `구분` + `조사완료 사례수` 를 포함하는 헤더 Field 의 인덱스를 찾는다(공백 제거 비교). 없으면 nil.
2. 헤더 다음 행부터 순회:
   - 값이 정확히 숫자 2개가 아니면(예: `조사방법1`) **블록 종료**.
   - labels==['전체'] → `Total`.
   - len(labels)==2 → 새 `Crosstab{Dimension: labels[0]}`, cell{Category: labels[1], 값}.
   - len(labels)==1 → 현재 Crosstab 에 cell{Category: labels[0], 값} (현재 Crosstab 없으면 무시).
3. 가중방법: labels 가 `기본가중`/`산출방법` 인 Field 의 첫 값. 표본오차: `표본오차` Field 의 첫 값.
4. 숫자 파싱은 콤마 제거 후 정수(비숫자면 그 행 skip).

## 5. CLI

`nesdc show <nttId> --crosstab`:
- 플래그 없으면 기존 동작(Detail 전체).
- `--crosstab` 면 `SampleCompositionOf(detail)` 결과만 출력(json/jsonl/table).
- nil(블록 없음)이면 stderr 안내 + 빈 출력.
- table: 차원별로 `차원 | 범주 | 완료 | 가중` 행, 맨 위 전체, 아래 가중방법·표본오차 주석.

## 6. 에러 처리

- 표본 구성 헤더 없음(일부 게시판/오래된 양식): `SampleCompositionOf` 가 nil, CLI 가 "표본 구성
  교차표를 찾지 못했습니다" stderr.
- 값이 한쪽만 숫자(완료만, 가중 빈칸): 가능한 쪽만 채우고 다른 쪽 0 — 단, 둘 다 비숫자면 종료.
- 반복 블록(다조사 survey): 첫 표본 구성 블록만 구조화(YAGNI). 필요 시 후속 확장.

## 7. 테스트

- `internal/nesdc/composition_test.go`(또는 기존 test 파일): 인라인 `[]Field` 픽스처(정찰 시퀀스
  모사)로 `SampleCompositionOf` 검증 — 전체/성별/연령/지역 차원 분리, 완료·가중 값, 가중방법·표본오차.
- 헤더 없는 Fields → nil.
- 기존 `results_view.html` 픽스처로 end-to-end(파싱→구조화) 한 케이스.

## 8. 비범위

- PDF 정당지지율(이념·연령별) 추출 → **후속 Phase**(기관별 레이아웃, OCR/테이블 추출).
- 구성 비율(%) 표는 실측상 대개 빈칸 → 이번엔 완료·가중 사례수만. (% 가 채워진 게시물 발견 시 확장.)
- 어떤 대표성·과대가중 "판단"도 없음(중립성).
