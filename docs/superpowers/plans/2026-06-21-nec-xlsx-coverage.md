# Phase 2: NEC XLSX 개표 완전 커버리지 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 지방선거 등 XLSX 개표결과를, CSV와 하나의 공통 `ElectionResult` 스키마로 — 선거별 손매핑 없이 헤더 라벨 기반으로 — 정규화한다.

**Architecture:** 신규 `election.go`(공통 타입 + 파생 규칙), `xlsx.go`(excelize 멀티시트 wide→long 파서, 앵커 라벨 기반 열 감지), `results.go`에 CSV→공통 어댑터. CLI는 `nec results`가 CSV/XLSX를 매직바이트로 판별. 모두 순수 파싱 + 기존 다운로드 재사용.

**Tech Stack:** Go, excelize/v2 (이미 의존성), 표준 라이브러리. 테스트는 `go test`, XLSX 픽스처는 테스트에서 excelize로 생성.

## Global Constraints

- **중립성(타협 불가):** 플래그·점수·"이상치"·"검증 결과"·해석 금지. 원자료 완전 보존 + 정의 명시 표준 파생만. `aggregate`는 소스의 합계/소계 라벨만 읽는다(우리 해석 아님).
- **선거별 손매핑 금지:** 차원 열은 고정 필드로 매핑하지 않고 `Dimension{Label,Value}`로 라벨 그대로 캡처. 매핑 대상은 안정적 앵커 라벨뿐: `선거인수`·`투표수`·`후보자별 득표수`·`계`·`무효투표수`·`기권수`. 앵커 없는 시트는 skip + stderr 경고(추측 매핑 금지).
- **파생 규칙(고정):** 구분(공백제거) `합계`/`소계`→ voteType `""`, aggregate true. `거소투표`→`거소`, `관외사전투표`→`관외사전`, `관내사전투표`→`관내사전`, 그 외→`본투표` (모두 aggregate false).
- **하위호환:** 기존 `ResultRecord`(CSV)와 P1 `Aggregate`는 그대로 동작. CSV→공통은 어댑터.
- 후보: `CandidateVote{Party,Name,Votes}` 재사용. 비례=Name "", 교육감=Party "". row2 후보정의 `정당\n이름` 분리(`\n` 기준; 줄바꿈 없으면 전체를 Name 또는 Party 중 하나로 — §Task2 규칙).
- 모듈 `github.com/JungHoonGhae/kvote`. 출력은 `internal/output` 재사용. 숫자는 천단위 콤마 제거(`atoiLoose` 재사용).

---

### Task 1: 공통 타입 + 파생 규칙 (election.go)

**Files:**
- Create: `internal/nec/election.go`
- Test: `internal/nec/nec_test.go`

**Interfaces:**
- Produces:
  - `type Dimension struct { Label, Value string }` (json `label`,`value`).
  - `type ElectionResult struct { Race string; Dimensions []Dimension; VoteType string; Aggregate bool; Electorate, Votes, Invalid, Abstention int; Candidates []CandidateVote }` (json: race, dimensions, voteType, aggregate, electorate, votes, invalid, abstention, candidates).
  - `func (e ElectionResult) Dim(label string) string` — 라벨로 차원 값 조회(없으면 "").
  - `func deriveVoteType(gubun string) (voteType string, aggregate bool)` — 패키지 내부.

- [ ] **Step 1: Write the failing test**

`internal/nec/nec_test.go`에 추가:

```go
func TestDeriveVoteType(t *testing.T) {
	cases := []struct {
		gubun, wantVT string
		wantAgg       bool
	}{
		{"합계", "", true},
		{"소계", "", true},
		{"거소투표", "거소", false},
		{"관외사전투표", "관외사전", false},
		{"관내사전투표", "관내사전", false},
		{"무효 투표수", "본투표", false}, // 공백 포함 임의값도 default
		{"청운효자동", "본투표", false},
	}
	for _, c := range cases {
		vt, agg := deriveVoteType(c.gubun)
		if vt != c.wantVT || agg != c.wantAgg {
			t.Errorf("deriveVoteType(%q) = (%q,%v), want (%q,%v)", c.gubun, vt, agg, c.wantVT, c.wantAgg)
		}
	}
}

func TestElectionResultDim(t *testing.T) {
	e := ElectionResult{Dimensions: []Dimension{{"시도명", "서울"}, {"구분", "소계"}}}
	if e.Dim("시도명") != "서울" {
		t.Errorf("Dim(시도명) = %q", e.Dim("시도명"))
	}
	if e.Dim("없는라벨") != "" {
		t.Errorf("missing label should be empty, got %q", e.Dim("없는라벨"))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nec -run 'TestDeriveVoteType|TestElectionResultDim' -v`
Expected: FAIL — `undefined: deriveVoteType` / `ElectionResult`

- [ ] **Step 3: Create election.go**

`internal/nec/election.go`:

```go
package nec

import "strings"

// Dimension is one source column to the left of the 선거인수 anchor, kept under
// its verbatim header label. The set/order varies by election type and year; we
// capture whatever is there rather than mapping each layout by hand.
type Dimension struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ElectionResult is the common, election-type-aware normalized row that both the
// CSV (총선·대선) and XLSX (지방선거 등) parsers converge to.
type ElectionResult struct {
	Race       string          `json:"race"`
	Dimensions []Dimension     `json:"dimensions"`
	VoteType   string          `json:"voteType"`
	Aggregate  bool            `json:"aggregate"`
	Electorate int             `json:"electorate"`
	Votes      int             `json:"votes"`
	Invalid    int             `json:"invalid"`
	Abstention int             `json:"abstention"`
	Candidates []CandidateVote `json:"candidates"`
}

// Dim returns the value of the dimension with the given label, or "".
func (e ElectionResult) Dim(label string) string {
	for _, d := range e.Dimensions {
		if d.Label == label {
			return d.Value
		}
	}
	return ""
}

// deriveVoteType maps a 구분 value to a vote-type label and the aggregate flag.
// aggregate reads only the source's own subtotal labels (합계/소계) — it embeds
// no interpretation of ours.
func deriveVoteType(gubun string) (voteType string, aggregate bool) {
	switch strings.ReplaceAll(gubun, " ", "") {
	case "합계", "소계":
		return "", true
	case "거소투표":
		return "거소", false
	case "관외사전투표":
		return "관외사전", false
	case "관내사전투표":
		return "관내사전", false
	default:
		return "본투표", false
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/nec -run 'TestDeriveVoteType|TestElectionResultDim' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nec/election.go internal/nec/nec_test.go
git commit -m "feat(nec): 공통 ElectionResult 타입 + voteType/aggregate 파생"
```

---

### Task 2: XLSX 파서 (xlsx.go)

**Files:**
- Create: `internal/nec/xlsx.go`
- Test: `internal/nec/nec_test.go`

**Interfaces:**
- Consumes: `ElectionResult`, `Dimension`, `deriveVoteType` (Task 1); `CandidateVote`, `atoiLoose` (기존).
- Produces: `func ParseResultsXLSX(raw []byte) ([]ElectionResult, error)`.

**Algorithm (구현은 이 알고리즘으로 테스트를 통과시킨다):**
1. `excelize.OpenReader(bytes.NewReader(raw))` → 전 시트(`GetSheetList`) 순회. 시트명 = `Race`.
2. 각 시트 `GetRows`. row0(라벨)에서 앵커 인덱스 탐색:
   - `electIdx` = `선거인수` 첫 등장 열. 없으면 그 시트 skip + `fmt.Fprintf(os.Stderr, ...)` 경고, continue.
   - `votesIdx` = electIdx 이후 첫 `투표수`. `candStart` = `후보자별 득표수` 라벨 열. `gyeIdx`=`계`, `invalidIdx`=`무효투표수`, `abstIdx`=`기권수`. (공백 제거 비교.)
   - `candEnd` = `gyeIdx` (후보 블록은 [candStart, gyeIdx)).
   - 차원 열 = [0, electIdx) — 각 열 라벨 = row0[i] (공백 trim).
   - `gubunDimIdx` = 차원 열 중 라벨이 `구분` 인 인덱스(없을 수도; 그러면 gubun="").
3. row 인덱스 2부터(0=라벨,1=병합잔재) 데이터:
   - 후보정의행 판정: 차원 열의 "읍면동명"·"구분" 값이 모두 빈칸이고 후보 열에 비어있지 않은 텍스트가 있으면 → **후보 헤더 갱신**: 각 후보 열 j의 `정당\n이름` 파싱(`\n` 기준 split; 줄바꿈 없으면 정당으로 간주하되 이름칸 비움 — 단 교육감 시트는 정당 없이 이름만이라, "정당\n이름"에 `\n` 없으면 Name 으로 둔다 → **규칙: split('\n') 후 두 토막이면 (party,name), 한 토막이면 (\"\", that)**). 이 행은 레코드로 내지 않고 continue.
     - 빈 후보 열은 그 선거구의 후보 수보다 많은 열 → nil 후보로 두고 그 열은 이후 무시.
   - 일반 데이터행: `ElectionResult` 생성.
     - `Dimensions` = [0,electIdx) 각 열 {Label: row0[i], Value: trim(row[i])} (값이 빈 차원도 포함해 형태 유지).
     - gubun = (gubunDimIdx 있으면 그 값 else "").
     - `VoteType, Aggregate = deriveVoteType(gubun)`.
     - `Electorate=atoiLoose(row[electIdx])`, `Votes=atoiLoose(row[votesIdx])`, `Invalid=atoiLoose(row[invalidIdx])`, `Abstention=atoiLoose(row[abstIdx])`.
     - 후보: candHeader[j] 가 정의된 각 열 j∈[candStart,candEnd) 에 대해, 값 v=trim(row[j]); v=="" 면 skip, 아니면 `CandidateVote{Party:candHeader[j].Party, Name:candHeader[j].Name, Votes:atoiLoose(v)}`.
   - 후보정의행을 한 번도 못 본 상태의 데이터행(이론상 없음)은 후보 빈 채로.
4. 빈 행(모든 셀 빈) skip.

- [ ] **Step 1: Write the failing test**

`internal/nec/nec_test.go`에 추가. 테스트가 픽스처 XLSX를 excelize로 만든다:

```go
func buildXLSX(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	// 시트1: 후보형 (시도지사 모사) — 차원 4열: 선거구명,구시군명,읍면동명,구분
	s1 := "시·도지사"
	f.SetSheetName(f.GetSheetName(0), s1)
	put := func(sheet string, rows [][]any) {
		for i, row := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if err := f.SetSheetRow(sheet, cell, &row); err != nil {
				t.Fatal(err)
			}
		}
	}
	put(s1, [][]any{
		{"선거구명", "구시군명", "읍면동명", "구분", "선거인수", "투표수", "후보자별 득표수", "", "계", "무효투표수", "기권수"},
		{"", "", "", "", "선거인수", "선거인수", "후보1", "후보2", "", "", ""},
		{"서울특별시", "종로구", "", "", "", "", "더불어민주당\n송영길", "국민의힘\n오세훈", "", "", ""},
		{"서울특별시", "종로구", "", "합계", "1,000", "700", "300", "390", "690", "10", "300"},
		{"서울특별시", "종로구", "", "관외사전투표", "100", "100", "44", "54", "98", "2", "0"},
		{"서울특별시", "종로구", "청운효자동", "소계", "500", "400", "180", "210", "390", "10", "100"},
		{"서울특별시", "종로구", "청운효자동", "관내사전투표", "200", "200", "90", "108", "198", "2", "0"},
	})
	// 시트2: 비례형 — 차원 3열, 정당만(줄바꿈 없음)
	s2 := "광역의원비례대표"
	f.NewSheet(s2)
	put(s2, [][]any{
		{"시도명", "구시군명", "읍면동명", "구분", "선거인수", "투표수", "후보자별 득표수", "", "계", "무효투표수", "기권수"},
		{"", "", "", "", "선거인수", "선거인수", "정당1", "정당2", "", "", ""},
		{"서울특별시", "종로구", "", "", "", "", "더불어민주당", "국민의힘", "", "", ""},
		{"서울특별시", "종로구", "", "합계", "1,000", "700", "320", "360", "680", "20", "300"},
	})
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseResultsXLSX(t *testing.T) {
	recs, err := ParseResultsXLSX(buildXLSX(t))
	if err != nil {
		t.Fatalf("ParseResultsXLSX: %v", err)
	}
	// 시트1 데이터행 4개(합계/관외사전/소계/관내사전) + 시트2 1개(합계) = 5; 후보정의행은 제외
	if len(recs) != 5 {
		t.Fatalf("got %d records, want 5: %+v", len(recs), recs)
	}
	// 첫 레코드: 시도지사 합계
	r := recs[0]
	if r.Race != "시·도지사" {
		t.Errorf("race = %q", r.Race)
	}
	if r.Dim("선거구명") != "서울특별시" || r.Dim("구시군명") != "종로구" || r.Dim("구분") != "합계" {
		t.Errorf("dimensions wrong: %+v", r.Dimensions)
	}
	if !r.Aggregate || r.VoteType != "" {
		t.Errorf("합계 should be aggregate w/ empty voteType: agg=%v vt=%q", r.Aggregate, r.VoteType)
	}
	if r.Electorate != 1000 || r.Votes != 700 || r.Invalid != 10 || r.Abstention != 300 {
		t.Errorf("metrics wrong: %+v", r)
	}
	if len(r.Candidates) != 2 || r.Candidates[0].Party != "더불어민주당" || r.Candidates[0].Name != "송영길" || r.Candidates[0].Votes != 300 {
		t.Errorf("candidates wrong: %+v", r.Candidates)
	}
	// 관내사전투표 leaf
	var leaf *ElectionResult
	for i := range recs {
		if recs[i].Dim("구분") == "관내사전투표" && recs[i].Race == "시·도지사" {
			leaf = &recs[i]
		}
	}
	if leaf == nil || leaf.VoteType != "관내사전" || leaf.Aggregate {
		t.Fatalf("관내사전 leaf wrong: %+v", leaf)
	}
	// 비례: 정당만, Name 빈칸
	var prop *ElectionResult
	for i := range recs {
		if recs[i].Race == "광역의원비례대표" {
			prop = &recs[i]
		}
	}
	if prop == nil || len(prop.Candidates) != 2 || prop.Candidates[0].Party != "더불어민주당" || prop.Candidates[0].Name != "" {
		t.Fatalf("비례 candidate wrong: %+v", prop)
	}
}

func TestParseResultsXLSXSkipsUnanchored(t *testing.T) {
	f := excelize.NewFile()
	f.SetSheetName(f.GetSheetName(0), "엉뚱시트")
	cell, _ := excelize.CoordinatesToCellName(1, 1)
	row := []any{"아무거나", "헤더", "여기"}
	f.SetSheetRow("엉뚱시트", cell, &row)
	f.NewSheet("시·도지사")
	good := [][]any{
		{"선거구명", "구시군명", "읍면동명", "구분", "선거인수", "투표수", "후보자별 득표수", "계", "무효투표수", "기권수"},
		{"", "", "", "", "", "", "후보1", "", "", ""},
		{"서울특별시", "종로구", "", "", "", "", "무소속\n홍길동", "", "", ""},
		{"서울특별시", "종로구", "", "합계", "10", "8", "8", "8", "0", "2"},
	}
	for i, r := range good {
		c, _ := excelize.CoordinatesToCellName(1, i+1)
		f.SetSheetRow("시·도지사", c, &r)
	}
	buf, _ := f.WriteToBuffer()
	recs, err := ParseResultsXLSX(buf.Bytes())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(recs) != 1 || recs[0].Race != "시·도지사" {
		t.Errorf("unanchored sheet should be skipped, valid one parsed: %+v", recs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nec -run TestParseResultsXLSX -v`
Expected: FAIL — `undefined: ParseResultsXLSX`

- [ ] **Step 3: Implement xlsx.go to pass the tests (follow the Algorithm above)**

Create `internal/nec/xlsx.go`. Implement `ParseResultsXLSX(raw []byte) ([]ElectionResult, error)` exactly per the Algorithm block above. Use `github.com/xuri/excelize/v2`. Key points the tests pin down: skip rows 0–1; treat a row whose 읍면동명/구분 dims are empty AND candidate cells non-empty as a candidate-definition row (update header, don't emit); split candidate header on `\n` → 2 parts=(party,name), 1 part=("",that); build `Dimensions` from columns [0,선거인수); derive voteType/aggregate from the 구분 dim; read metrics by anchor label index; emit a candidate per non-empty candidate cell with a defined header. Skip wholly-empty rows. If a sheet lacks the 선거인수 or 후보자별 득표수 anchor, `fmt.Fprintf(os.Stderr, "skip sheet %q: missing anchor\n", name)` and continue; return an error only if no sheet yielded any record.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/nec -run TestParseResultsXLSX -v`
Expected: PASS (both)

- [ ] **Step 5: Run full suite + build**

Run: `go test ./... && make build`
Expected: all `ok`, build success.

- [ ] **Step 6: Commit**

```bash
git add internal/nec/xlsx.go internal/nec/nec_test.go
git commit -m "feat(nec): XLSX 개표결과 파서 (멀티시트 wide→long, 앵커 라벨 기반)"
```

---

### Task 3: CSV→공통 어댑터 + CLI 통합

**Files:**
- Modify: `internal/nec/results.go` (`ToElectionResult` 어댑터)
- Modify: `cmd/kvote/nec.go` (results 명령에 XLSX 판별 + --race/--leaf-only + renderElection)
- Test: `internal/nec/nec_test.go`

**Interfaces:**
- Consumes: `ParseResultsXLSX` (Task 2), `ElectionResult`/`Dimension` (Task 1), `ResultRecord`, `ParseResults` (기존).
- Produces: `func (r ResultRecord) ToElectionResult() ElectionResult`.

- [ ] **Step 1: Write the failing test (adapter)**

`internal/nec/nec_test.go`에 추가:

```go
func TestResultRecordToElectionResult(t *testing.T) {
	r := ResultRecord{
		Sido: "서울", District: "종로구", Town: "청운효자동", Booth: "관내사전투표",
		VoteType: "관내사전", Electorate: 100, Votes: 90, Invalid: 2, Abstention: 10,
		Candidates: []CandidateVote{{"A당", "김갑", 50}},
	}
	e := r.ToElectionResult()
	if e.Dim("시도명") != "서울" || e.Dim("선거구명") != "종로구" || e.Dim("법정읍면동명") != "청운효자동" || e.Dim("투표구명") != "관내사전투표" {
		t.Errorf("dimensions wrong: %+v", e.Dimensions)
	}
	if e.VoteType != "관내사전" || e.Aggregate {
		t.Errorf("voteType/aggregate wrong: %q %v", e.VoteType, e.Aggregate)
	}
	if e.Electorate != 100 || len(e.Candidates) != 1 || e.Candidates[0].Name != "김갑" {
		t.Errorf("fields wrong: %+v", e)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nec -run TestResultRecordToElectionResult -v`
Expected: FAIL — `ToElectionResult undefined`

- [ ] **Step 3: Add the adapter in results.go**

`internal/nec/results.go` 끝에 추가:

```go
// ToElectionResult maps a CSV-derived ResultRecord into the common schema. CSV
// rows are all leaves (no 합계/소계), so Aggregate is always false.
func (r ResultRecord) ToElectionResult() ElectionResult {
	return ElectionResult{
		Race: "",
		Dimensions: []Dimension{
			{"시도명", r.Sido},
			{"선거구명", r.District},
			{"법정읍면동명", r.Town},
			{"투표구명", r.Booth},
		},
		VoteType:   r.VoteType,
		Aggregate:  false,
		Electorate: r.Electorate,
		Votes:      r.Votes,
		Invalid:    r.Invalid,
		Abstention: r.Abstention,
		Candidates: r.Candidates,
	}
}
```

- [ ] **Step 4: Run adapter test**

Run: `go test ./internal/nec -run TestResultRecordToElectionResult -v`
Expected: PASS

- [ ] **Step 5: Wire CLI — XLSX detection + flags + renderer**

In `cmd/kvote/nec.go` `necResultsCmd`: after computing `raw` (the file bytes) and BEFORE the existing `nec.ParseResults(raw)` block, branch on file type. Replace the block:

```go
			recs, err := nec.ParseResults(raw)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "정규화 완료: %d개 투표구\n", len(recs))

			level, ok := parseAggLevel(aggregate)
			if !ok {
				return fmt.Errorf("알 수 없는 --aggregate 값 %q (none|town|sgg|sido|national)", aggregate)
			}
			if level == nec.AggNone {
				return renderResults(cmd, format, recs)
			}
			aggs := nec.Aggregate(recs, level, byVoteType)
			fmt.Fprintf(cmd.ErrOrStderr(), "집계 완료: %d개 그룹 (level=%s)\n", len(aggs), aggregate)
			return renderAggregated(cmd, format, aggs)
```

with:

```go
			if isXLSX(raw) {
				ers, err := nec.ParseResultsXLSX(raw)
				if err != nil {
					return err
				}
				if race != "" {
					ers = filterRace(ers, race)
				}
				if leafOnly {
					ers = filterLeaf(ers)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "정규화 완료: %d개 행 (XLSX)\n", len(ers))
				return renderElection(cmd, format, ers)
			}

			recs, err := nec.ParseResults(raw)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "정규화 완료: %d개 투표구\n", len(recs))

			level, ok := parseAggLevel(aggregate)
			if !ok {
				return fmt.Errorf("알 수 없는 --aggregate 값 %q (none|town|sgg|sido|national)", aggregate)
			}
			if level == nec.AggNone {
				return renderResults(cmd, format, recs)
			}
			aggs := nec.Aggregate(recs, level, byVoteType)
			fmt.Fprintf(cmd.ErrOrStderr(), "집계 완료: %d개 그룹 (level=%s)\n", len(aggs), aggregate)
			return renderAggregated(cmd, format, aggs)
```

Add flag vars at the top of `necResultsCmd` (extend the existing `var file, aggregate string` / `var byVoteType bool`):

```go
		var race string
		var leafOnly bool
```

Register flags after the existing ones:

```go
	c.Flags().StringVar(&race, "race", "", "XLSX 선거종류(시트명) 부분일치 필터")
	c.Flags().BoolVar(&leafOnly, "leaf-only", false, "XLSX 집계행(합계/소계) 제외, leaf만")
```

Add helpers near `parseAggLevel` in `cmd/kvote/nec.go`:

```go
// isXLSX reports whether raw is an XLSX (zip) file by its magic bytes.
func isXLSX(raw []byte) bool {
	return len(raw) >= 2 && raw[0] == 'P' && raw[1] == 'K'
}

func filterRace(ers []nec.ElectionResult, race string) []nec.ElectionResult {
	out := ers[:0:0]
	for _, e := range ers {
		if strings.Contains(e.Race, race) {
			out = append(out, e)
		}
	}
	return out
}

func filterLeaf(ers []nec.ElectionResult) []nec.ElectionResult {
	out := ers[:0:0]
	for _, e := range ers {
		if !e.Aggregate {
			out = append(out, e)
		}
	}
	return out
}

func renderElection(cmd *cobra.Command, format output.Format, ers []nec.ElectionResult) error {
	switch format {
	case output.JSON:
		return output.WriteJSON(cmd.OutOrStdout(), ers)
	case output.JSONL:
		items := make([]any, len(ers))
		for i := range ers {
			items[i] = ers[i]
		}
		return output.WriteJSONL(cmd.OutOrStdout(), items)
	default:
		headers := []string{"race", "차원", "투표유형", "집계", "선거인수", "투표수", "무효", "후보수"}
		rows := make([][]string, 0, len(ers))
		for _, e := range ers {
			dims := make([]string, 0, len(e.Dimensions))
			for _, d := range e.Dimensions {
				if d.Value != "" {
					dims = append(dims, d.Value)
				}
			}
			rows = append(rows, []string{
				e.Race, strings.Join(dims, ">"), e.VoteType, fmt.Sprint(e.Aggregate),
				fmt.Sprint(e.Electorate), fmt.Sprint(e.Votes), fmt.Sprint(e.Invalid), fmt.Sprint(len(e.Candidates)),
			})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}
```

Ensure `strings` is imported in `cmd/kvote/nec.go` (add to import block if missing).

- [ ] **Step 6: Build + full tests + live verify**

Run:
```bash
make build && go test ./...
XLSX=$(ls /tmp/necxlsx/*.xlsx | head -1)
./bin/kvote -f jsonl nec results --file "$XLSX" --race 교육감 --leaf-only 2>/dev/null | head -2 | \
  python3 -c "import sys,json; [print(json.loads(l)['race'], json.loads(l)['voteType'], [d['value'] for d in json.loads(l)['dimensions']]) for l in sys.stdin]"
```
Expected: build+tests green; 교육감 leaf 행들이 race=교육감, voteType∈{본투표,관내사전,거소,관외사전}, aggregate=false 로 출력.

- [ ] **Step 7: Commit**

```bash
git add internal/nec/results.go cmd/kvote/nec.go
git commit -m "feat(nec): CSV→공통 어댑터 + nec results XLSX 판별/--race/--leaf-only"
```

---

### Task 4: 문서

**Files:**
- Modify: `README.md` (NEC 예시 + 데이터 출처 표에 XLSX)
- Modify: `CLAUDE.md` (internal/nec 아키텍처에 election.go/xlsx.go)

- [ ] **Step 1: README 예시 추가**

`README.md` NEC 예시 블록의 `--aggregate sido` 줄 다음에 추가:

```bash
# XLSX 지방선거 개표결과 — 공통 스키마(선거종류·차원 라벨 보존)
kvote nec pull 15101509 -o ./downloads                       # 제8회 지방선거 XLSX 원본
kvote nec results --file ./downloads/*.xlsx --race 교육감 --leaf-only -f jsonl
```

- [ ] **Step 2: CLAUDE.md 아키텍처 갱신**

`CLAUDE.md` `internal/nec/` 블록의 `results.go` 줄 다음에 추가:

```
  election.go       CSV·XLSX 공통 스키마(ElectionResult) + voteType/aggregate 파생
  xlsx.go           XLSX 멀티시트 wide→long 파서 (앵커 라벨 기반, 선거별 손매핑 없음)
```

- [ ] **Step 3: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs(nec): XLSX 커버리지 사용 예시 + election.go/xlsx.go 아키텍처"
```

---

## Self-Review

**Spec coverage:**
- 공통 ElectionResult + 라벨 기반 Dimensions → Task 1 ✓
- voteType/aggregate 파생(합계/소계만 aggregate) → Task 1 (deriveVoteType) ✓
- XLSX 멀티시트 wide→long, 앵커 라벨 감지, 후보 재정의 추적, 비례/교육감 후보형 → Task 2 ✓
- 앵커 없는 시트 skip+경고 → Task 2 (TestParseResultsXLSXSkipsUnanchored) ✓
- CSV→공통 어댑터(하위호환 유지, ResultRecord/Aggregate 그대로) → Task 3 ✓
- CLI XLSX 판별 + --race/--leaf-only → Task 3 ✓
- 문서 → Task 4 ✓
- 비범위(XLSX→Aggregate 연동, 이상치 판단 없음) → 계획에 없음 ✓

**Placeholder scan:** 모든 step에 실제 코드/명령. Task 2 Step 3은 알고리즘 명세 + 완전한 테스트로 행위를 고정(파서 본문은 구현자가 테스트 통과로 작성) — 의도된 형태.

**Type consistency:** `ElectionResult`/`Dimension`/`deriveVoteType`(T1) → `ParseResultsXLSX`(T2) → `ToElectionResult`/CLI `renderElection`/`filterRace`/`filterLeaf`/`isXLSX`(T3). `CandidateVote`(기존), `atoiLoose`(기존) 재사용. 필드명(Race/Dimensions/VoteType/Aggregate/Electorate/Votes/Invalid/Abstention/Candidates) 전 태스크 일치 ✓.
