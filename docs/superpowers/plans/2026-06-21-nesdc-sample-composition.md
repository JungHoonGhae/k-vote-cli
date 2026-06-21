# Phase 4: NESDC 표본 구성 교차표 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** NESDC 상세의 `Detail.Fields`에 들어있는 표본 구성 교차표(성별·연령·지역 × 완료·가중 사례수)를 구조화해 `nesdc show --crosstab`으로 노출한다.

**Architecture:** 신규 `internal/nesdc/composition.go`의 순수 함수 `SampleCompositionOf(*Detail)`가 이미 파싱된 Fields 위에서 동작(네트워크 무관). `Detail` 구조는 불변(lossless). CLI는 `nesdc show`에 `--crosstab` 플래그를 더한다.

**Tech Stack:** Go 표준 라이브러리만(신규 의존성 없음). 테스트는 `go test`, 인라인 `[]Field` 픽스처.

## Global Constraints

- **중립성(타협 불가):** 원자료(완료·가중 사례수) 그대로 보존, 구조화만. 비율·격차·대표성·과대가중 판단/플래그/점수 금지.
- **lossless 유지:** `Detail`/`Detail.Fields`는 수정하지 않는다. 교차표는 파생이므로 별도 함수.
- **파싱 규칙(고정):** 헤더(`구분`+`조사완료 사례수` 포함) 다음부터, 값이 숫자 2개가 아니면 블록 종료. labels==['전체']→Total. len(labels)==2→새 차원[0]+범주[1]. len(labels)==1→현재 차원에 범주 연속. 숫자는 콤마 제거 후 정수.
- **반복 블록:** 첫 표본 구성 블록만(YAGNI).
- 모듈 `github.com/JungHoonGhae/kvote`. `internal/nesdc`의 `Field{Labels []string; Values []string}` 사용. 출력은 `internal/output` 재사용.

---

### Task 1: 표본 구성 파서 (composition.go)

**Files:**
- Create: `internal/nesdc/composition.go`
- Test: `internal/nesdc/nesdc_test.go`

**Interfaces:**
- Consumes: 기존 `Detail{Fields []Field}`, `Field{Labels []string; Values []string}` (internal/nesdc).
- Produces:
  - `type CompositionCell struct { Category string; Completed, Weighted int }` (json: category, completed, weighted).
  - `type Crosstab struct { Dimension string; Cells []CompositionCell }` (json: dimension, cells).
  - `type SampleComposition struct { Total *CompositionCell; Crosstabs []Crosstab; Weighting, MarginError string }` (json: total(omitempty), crosstabs, weighting(omitempty), marginError(omitempty)).
  - `func SampleCompositionOf(d *Detail) *SampleComposition` — 블록 없으면 nil.

- [ ] **Step 1: Write the failing test**

`internal/nesdc/nesdc_test.go`에 추가:

```go
func sampleCompFields() []Field {
	return []Field{
		{Labels: []string{"표본의 크기"}},
		{Labels: []string{"구분", "조사완료 사례수(명)", "가중값 적용 기준 사례수(명)"}},
		{Labels: []string{"전체"}, Values: []string{"1,001", "1001"}},
		{Labels: []string{"성별", "남"}, Values: []string{"546", "496"}},
		{Labels: []string{"여"}, Values: []string{"455", "505"}},
		{Labels: []string{"연령대별", "18~29세"}, Values: []string{"128", "149"}},
		{Labels: []string{"70세 이상"}, Values: []string{"153", "162"}},
		{Labels: []string{"지역별", "서울"}, Values: []string{"198", "185"}},
		{Labels: []string{"제주"}, Values: []string{"14", "12"}},
		{Labels: []string{"조사방법1"}, Values: []string{"무선 ARS"}}, // 블록 종료 트리거
		{Labels: []string{"기본가중", "산출방법"}, Values: []string{"성별·연령별·지역별 가중값 부여"}},
		{Labels: []string{"표본오차"}, Values: []string{"95% 신뢰수준에 ±3.1%P"}},
	}
}

func TestSampleCompositionOf(t *testing.T) {
	sc := SampleCompositionOf(&Detail{Fields: sampleCompFields()})
	if sc == nil {
		t.Fatal("expected a SampleComposition, got nil")
	}
	if sc.Total == nil || sc.Total.Completed != 1001 || sc.Total.Weighted != 1001 {
		t.Errorf("total wrong: %+v", sc.Total)
	}
	if len(sc.Crosstabs) != 3 {
		t.Fatalf("got %d crosstabs, want 3 (성별/연령대별/지역별): %+v", len(sc.Crosstabs), sc.Crosstabs)
	}
	g := sc.Crosstabs[0]
	if g.Dimension != "성별" || len(g.Cells) != 2 || g.Cells[0].Category != "남" || g.Cells[0].Completed != 546 || g.Cells[0].Weighted != 496 {
		t.Errorf("성별 crosstab wrong: %+v", g)
	}
	if g.Cells[1].Category != "여" || g.Cells[1].Completed != 455 {
		t.Errorf("성별 여 wrong: %+v", g.Cells[1])
	}
	if sc.Crosstabs[1].Dimension != "연령대별" || sc.Crosstabs[1].Cells[0].Category != "18~29세" {
		t.Errorf("연령 crosstab wrong: %+v", sc.Crosstabs[1])
	}
	if sc.Crosstabs[2].Dimension != "지역별" || sc.Crosstabs[2].Cells[1].Category != "제주" {
		t.Errorf("지역 crosstab wrong: %+v", sc.Crosstabs[2])
	}
	if sc.Weighting == "" || sc.MarginError != "95% 신뢰수준에 ±3.1%P" {
		t.Errorf("weighting/marginError wrong: %q / %q", sc.Weighting, sc.MarginError)
	}
}

func TestSampleCompositionOfNoBlock(t *testing.T) {
	d := &Detail{Fields: []Field{{Labels: []string{"조사기관명"}, Values: []string{"리얼미터"}}}}
	if sc := SampleCompositionOf(d); sc != nil {
		t.Errorf("expected nil when no composition block, got %+v", sc)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nesdc -run TestSampleComposition -v`
Expected: FAIL — `undefined: SampleCompositionOf`

- [ ] **Step 3: Create composition.go**

`internal/nesdc/composition.go`:

```go
package nesdc

import (
	"strconv"
	"strings"
)

// CompositionCell is one demographic category's sample counts.
type CompositionCell struct {
	Category  string `json:"category"`
	Completed int    `json:"completed"`
	Weighted  int    `json:"weighted"`
}

// Crosstab is one demographic dimension's breakdown of the sample.
type Crosstab struct {
	Dimension string            `json:"dimension"`
	Cells     []CompositionCell `json:"cells"`
}

// SampleComposition is the structured 표본 구성: who was sampled (completed vs
// weighted counts) by 성별/연령대별/지역별, plus the weighting method and margin
// of error. All raw counts are preserved; nothing is interpreted.
type SampleComposition struct {
	Total       *CompositionCell `json:"total,omitempty"`
	Crosstabs   []Crosstab       `json:"crosstabs"`
	Weighting   string           `json:"weighting,omitempty"`
	MarginError string           `json:"marginError,omitempty"`
}

// SampleCompositionOf derives the sample-composition crosstab from a detail
// page's already-parsed Fields. Returns nil when the block is absent.
func SampleCompositionOf(d *Detail) *SampleComposition {
	start := compositionHeaderIndex(d.Fields)
	if start < 0 {
		return nil
	}
	sc := &SampleComposition{}
	var cur *Crosstab
	for _, f := range d.Fields[start+1:] {
		completed, weighted, ok := twoInts(f.Values)
		if !ok {
			break // first non-(int,int) row ends the block
		}
		switch len(f.Labels) {
		case 0:
			continue
		case 1:
			if f.Labels[0] == "전체" {
				sc.Total = &CompositionCell{Category: "전체", Completed: completed, Weighted: weighted}
				continue
			}
			if cur != nil {
				cur.Cells = append(cur.Cells, CompositionCell{Category: f.Labels[0], Completed: completed, Weighted: weighted})
			}
		default: // >=2 labels: [dimension, category]
			sc.Crosstabs = append(sc.Crosstabs, Crosstab{Dimension: f.Labels[0]})
			cur = &sc.Crosstabs[len(sc.Crosstabs)-1]
			cur.Cells = append(cur.Cells, CompositionCell{Category: f.Labels[1], Completed: completed, Weighted: weighted})
		}
	}
	sc.Weighting = fieldValueWhereLabel(d.Fields, "산출방법")
	sc.MarginError = fieldValueWhereLabel(d.Fields, "표본오차")
	if sc.Total == nil && len(sc.Crosstabs) == 0 {
		return nil
	}
	return sc
}

func compositionHeaderIndex(fields []Field) int {
	for i, f := range fields {
		joined := strings.ReplaceAll(strings.Join(f.Labels, ""), " ", "")
		if strings.Contains(joined, "구분") && strings.Contains(joined, "조사완료사례수") {
			return i
		}
	}
	return -1
}

// twoInts parses exactly two integer values (comma-stripped). ok=false when the
// slice is not two parseable integers.
func twoInts(vals []string) (a, b int, ok bool) {
	if len(vals) != 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(vals[0]), ",", ""))
	b, err2 := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(vals[1]), ",", ""))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return a, b, true
}

// fieldValueWhereLabel returns the first value of the first field whose labels
// contain the given label, or "".
func fieldValueWhereLabel(fields []Field, label string) string {
	for _, f := range fields {
		for _, l := range f.Labels {
			if l == label && len(f.Values) > 0 {
				return f.Values[0]
			}
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/nesdc -run TestSampleComposition -v`
Expected: PASS (both)

- [ ] **Step 5: Run full suite**

Run: `go test ./...`
Expected: all `ok`

- [ ] **Step 6: Commit**

```bash
git add internal/nesdc/composition.go internal/nesdc/nesdc_test.go
git commit -m "feat(nesdc): 표본 구성 교차표 구조화 (SampleCompositionOf)"
```

---

### Task 2: CLI — nesdc show --crosstab

**Files:**
- Modify: `cmd/kvote/nesdc.go` (show 명령에 --crosstab + renderComposition)
- Test: 수동 라이브 검증

**Interfaces:**
- Consumes: `nesdc.SampleCompositionOf`, `nesdc.SampleComposition`/`Crosstab`/`CompositionCell` (Task 1); 기존 `nesdcShowCmd`, `renderDetail`.

- [ ] **Step 1: Inspect the current show command**

Run: `grep -n "func nesdcShowCmd" cmd/kvote/nesdc.go`
Read the function to see its flag var(s), client call, and how it calls `renderDetail`. It builds a `nesdc.Detail` (call it `d`) and calls `renderDetail(cmd, format, d)`.

- [ ] **Step 2: Add the --crosstab flag and branch**

In `nesdcShowCmd`, add a flag var alongside the existing board flag var:

```go
	var crosstab bool
```

Register it with the other flags (where `--board` is registered):

```go
	c.Flags().BoolVar(&crosstab, "crosstab", false, "표본 구성 교차표(성별·연령·지역 × 완료·가중)만 출력")
```

In the RunE, after the detail `d` is obtained and BEFORE the existing `return renderDetail(...)`, insert:

```go
			if crosstab {
				sc := nesdc.SampleCompositionOf(d)
				if sc == nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "표본 구성 교차표를 찾지 못했습니다.")
					return nil
				}
				return renderComposition(cmd, format, sc)
			}
```

(If `fmt` is not yet imported in the file, add it. It almost certainly is.)

- [ ] **Step 3: Add renderComposition helper**

Add near `renderDetail` in `cmd/kvote/nesdc.go`:

```go
func renderComposition(cmd *cobra.Command, format output.Format, sc *nesdc.SampleComposition) error {
	if format != output.Table {
		return output.WriteJSON(cmd.OutOrStdout(), sc)
	}
	w := cmd.OutOrStdout()
	rows := make([][]string, 0)
	if sc.Total != nil {
		rows = append(rows, []string{"전체", "", fmt.Sprint(sc.Total.Completed), fmt.Sprint(sc.Total.Weighted)})
	}
	for _, ct := range sc.Crosstabs {
		for _, c := range ct.Cells {
			rows = append(rows, []string{ct.Dimension, c.Category, fmt.Sprint(c.Completed), fmt.Sprint(c.Weighted)})
		}
	}
	if err := output.WriteTable(w, []string{"차원", "범주", "완료", "가중"}, rows); err != nil {
		return err
	}
	if sc.Weighting != "" {
		fmt.Fprintf(w, "\n가중: %s\n", sc.Weighting)
	}
	if sc.MarginError != "" {
		fmt.Fprintf(w, "표본오차: %s\n", sc.MarginError)
	}
	return nil
}
```

- [ ] **Step 4: Build + full tests**

Run: `make build && go test ./...`
Expected: build success, all `ok`.

- [ ] **Step 5: Live verification**

Run:
```bash
./bin/kvote -f json nesdc show 19366 --crosstab 2>/dev/null | python3 -c "
import sys,json
sc=json.load(sys.stdin)
print('total:', sc.get('total'))
for ct in sc.get('crosstabs',[]):
    print(' ', ct['dimension'], '→', [(c['category'],c['completed'],c['weighted']) for c in ct['cells'][:3]])
print('가중:', (sc.get('weighting') or '')[:30], '| 표본오차:', sc.get('marginError'))
"
```
Expected: total {completed,weighted}, 성별/연령대별/지역별 각 차원에 범주별 완료·가중 사례수, 가중방법·표본오차 출력. (포털 마크업이 그대로면 성공.)

- [ ] **Step 6: Commit**

```bash
git add cmd/kvote/nesdc.go
git commit -m "feat(nesdc): nesdc show --crosstab (표본 구성 교차표 출력)"
```

---

### Task 3: 문서

**Files:**
- Modify: `README.md` (nesdc show 예시에 --crosstab)
- Modify: `CLAUDE.md` (internal/nesdc 아키텍처에 composition.go)

- [ ] **Step 1: README**

`README.md`의 `kvote nesdc show 19366` 줄 다음에 추가:

```bash
kvote nesdc show 19366 --crosstab -f table   # 표본 구성(성별·연령·지역 × 완료·가중)
```

- [ ] **Step 2: CLAUDE.md**

`CLAUDE.md`의 `internal/nesdc/` 블록 `detail.go` 줄 다음에 추가:

```
  composition.go    Detail.Fields → 표본 구성 교차표(SampleComposition) 파생. 중립.
```

- [ ] **Step 3: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs(nesdc): --crosstab 예시 + composition.go 아키텍처"
```

---

## Self-Review

**Spec coverage:**
- 표본 구성 교차표 구조화(성별/연령/지역 × 완료·가중) → Task 1 ✓
- [dim,cat]/[cat] 라벨 규칙 + 전체 + 블록종료(숫자2개 아님) → Task 1 (SampleCompositionOf) ✓
- 가중방법·표본오차 → Task 1 (fieldValueWhereLabel) ✓
- 블록 없으면 nil → Task 1 (TestSampleCompositionOfNoBlock) ✓
- Detail lossless 유지(수정 안 함) → Task 1은 별도 함수만 추가 ✓
- CLI --crosstab → Task 2 ✓
- 문서 → Task 3 ✓
- 비범위(PDF 지지율, % 표, 판단) → 계획에 없음 ✓

**Placeholder scan:** 모든 step에 실제 코드/명령. Task 2 Step 1만 "현재 코드 확인"(읽기) — 의도된 컨텍스트 수집 단계.

**Type consistency:** `SampleComposition`/`Crosstab`/`CompositionCell`/`SampleCompositionOf`(T1) → CLI `renderComposition`/`--crosstab`(T2). 필드명(Total/Crosstabs/Weighting/MarginError, Dimension/Cells, Category/Completed/Weighted) 일치 ✓. 기존 `Field{Labels,Values}` 사용 ✓.
