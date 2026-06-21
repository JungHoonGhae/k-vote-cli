# Phase 1: 개표 분석 파라미터 (중립) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 이미 정규화된 NEC 개표결과 위에 투표유형 차원·다단계 집계·표준 파생 파라미터(투표율·득표율·유효투표수)를 중립적으로 더해, 소비자가 사전/본 등 어떤 단위로도 슬라이스할 수 있게 한다.

**Architecture:** `internal/nec/results.go` 의 `ResultRecord` 에 `VoteType` 을 추가(파싱 시 분류)하고, 순수 함수 `Aggregate` 를 담은 `internal/nec/aggregate.go` 를 신설한다. CLI 는 `nec results` 에 `--aggregate`/`--by-votetype` 플래그를 더해 같은 데이터의 집계 뷰를 노출한다. 네트워크 무관 — 모두 파싱된 레코드 위 변환.

**Tech Stack:** Go, 표준 라이브러리만 (신규 의존성 없음). 테스트는 `go test`.

## Global Constraints

- **중립성(타협 불가):** 플래그·점수·순위·"이상치"·"검증 결과"·해석 금지. 원자료 완전 보존 + 정의가 명시된 표준 파생값만. 파생 필드는 재현 가능한 산술이어야 한다.
- **파생값 정의(고정):** `유효투표수 = 투표수 − 무효투표수`; `투표율 = 투표수 / 선거인수`; `후보 득표율 = 후보득표 / 유효투표수`. 분모 0 이면 해당 파생값은 0 (NaN 금지).
- **후보 합산 범위:** `town`·`sgg` 레벨만 후보별 합계를 낸다. `sido`·`national` 은 지표만 합산하고 후보 목록은 비운다(후보가 선거구마다 다름).
- Go 버전·모듈 경로: `github.com/JungHoonGhae/kvote`. 출력은 `internal/output` 의 `WriteJSON/WriteJSONL/WriteTable` 재사용. 한글 표는 `WriteTable`(CJK 폭 보정).
- 테스트는 픽스처/인라인으로 네트워크 없이. 빌드 검증은 `make build`.

---

### Task 1: 투표유형(VoteType) 차원 추가

**Files:**
- Modify: `internal/nec/results.go` (ResultRecord 구조체 + 레코드 생성부 `:91` + 분류 헬퍼 추가)
- Test: `internal/nec/nec_test.go`

**Interfaces:**
- Consumes: 기존 `ResultRecord`, `ParseResults([]byte) ([]ResultRecord, error)`.
- Produces: `ResultRecord.VoteType string` (값: `본투표`·`관내사전`·`관외사전`·`거소선상`); 패키지 내부 `classifyVoteType(town, booth string) string`.

- [ ] **Step 1: Write the failing test**

`internal/nec/nec_test.go` 에 추가:

```go
func TestClassifyVoteType(t *testing.T) {
	cases := []struct {
		town, booth, want string
	}{
		{"청운효자동", "제1투", "본투표"},
		{"청운효자동", "관내사전투표", "관내사전"},
		{"관외사전투표", "", "관외사전"},
		{"거소·선상투표", "", "거소선상"},
		{"중앙동", "", "본투표"},
	}
	for _, c := range cases {
		if got := classifyVoteType(c.town, c.booth); got != c.want {
			t.Errorf("classifyVoteType(%q,%q) = %q, want %q", c.town, c.booth, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nec -run TestClassifyVoteType -v`
Expected: FAIL — `undefined: classifyVoteType`

- [ ] **Step 3: Add the VoteType field**

`internal/nec/results.go` 의 `ResultRecord` 구조체에서 `Booth` 줄 바로 다음에 추가:

```go
	Booth      string          `json:"booth,omitempty"`
	VoteType   string          `json:"voteType"`
	Electorate int             `json:"electorate"`
```

- [ ] **Step 4: Add classifyVoteType and set it during parsing**

`internal/nec/results.go` 끝부분(파일 맨 아래)에 헬퍼 추가:

```go
// classifyVoteType labels a polling unit by the structural meaning already
// present in the source columns. It is a neutral label, not a judgment:
//   - town "거소·선상투표"      → 거소선상
//   - town "관외사전투표"        → 관외사전
//   - booth "관내사전투표"       → 관내사전
//   - otherwise (real 읍면동/투표구) → 본투표
func classifyVoteType(town, booth string) string {
	switch {
	case town == "거소·선상투표":
		return "거소선상"
	case town == "관외사전투표":
		return "관외사전"
	case booth == "관내사전투표":
		return "관내사전"
	default:
		return "본투표"
	}
}
```

그리고 레코드 생성부(`internal/nec/results.go:91`)를 수정 — 기존:

```go
			cur = &ResultRecord{Sido: sido, District: dist, Town: town, Booth: booth}
```

다음으로 교체:

```go
			cur = &ResultRecord{Sido: sido, District: dist, Town: town, Booth: booth, VoteType: classifyVoteType(town, booth)}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/nec -run 'TestClassifyVoteType|TestParseResults' -v`
Expected: PASS (기존 ParseResults 테스트도 통과 — VoteType 추가는 비파괴적)

- [ ] **Step 6: Commit**

```bash
git add internal/nec/results.go internal/nec/nec_test.go
git commit -m "feat(nec): ResultRecord에 투표유형(VoteType) 차원 추가"
```

---

### Task 2: 다단계 집계 + 파생 파라미터 (aggregate.go)

**Files:**
- Create: `internal/nec/aggregate.go`
- Test: `internal/nec/nec_test.go`

**Interfaces:**
- Consumes: `ResultRecord` (Task 1, `VoteType` 포함), `CandidateVote{Party,Name string; Votes int}`.
- Produces:
  - `type AggLevel string` 와 상수 `AggNone="none"`, `AggTown="town"`, `AggSgg="sgg"`, `AggSido="sido"`, `AggNational="national"`.
  - `type CandidateShare struct { Party, Name string; Votes int; Share float64 }`.
  - `type AggregatedRecord struct { Level, Sido, District, Town, VoteType string; Electorate, Votes, Invalid, ValidVotes, Abstention int; Turnout float64; Candidates []CandidateShare }`.
  - `func Aggregate(recs []ResultRecord, level AggLevel, byVoteType bool) []AggregatedRecord`.

- [ ] **Step 1: Write the failing tests**

`internal/nec/nec_test.go` 에 추가:

```go
func sampleRecs() []ResultRecord {
	return []ResultRecord{
		{Sido: "서울", District: "종로구", Town: "청운효자동", Booth: "제1투", VoteType: "본투표",
			Electorate: 1000, Votes: 800, Invalid: 20, Abstention: 200,
			Candidates: []CandidateVote{{"A당", "김갑", 500}, {"B당", "이을", 280}}},
		{Sido: "서울", District: "종로구", Town: "삼청동", Booth: "제1투", VoteType: "본투표",
			Electorate: 500, Votes: 400, Invalid: 10, Abstention: 100,
			Candidates: []CandidateVote{{"A당", "김갑", 200}, {"B당", "이을", 190}}},
		{Sido: "서울", District: "종로구", Town: "청운효자동", Booth: "관내사전투표", VoteType: "관내사전",
			Electorate: 300, Votes: 290, Invalid: 0, Abstention: 10,
			Candidates: []CandidateVote{{"A당", "김갑", 100}, {"B당", "이을", 190}}},
	}
}

func TestAggregateSgg(t *testing.T) {
	out := Aggregate(sampleRecs(), AggSgg, false)
	if len(out) != 1 {
		t.Fatalf("got %d groups, want 1 선거구", len(out))
	}
	r := out[0]
	if r.Sido != "서울" || r.District != "종로구" || r.Town != "" {
		t.Errorf("dimensions wrong: %+v", r)
	}
	if r.Electorate != 1800 || r.Votes != 1490 || r.Invalid != 30 || r.Abstention != 310 {
		t.Errorf("metric sums wrong: %+v", r)
	}
	if r.ValidVotes != 1460 { // 1490 - 30
		t.Errorf("validVotes = %d, want 1460", r.ValidVotes)
	}
	if r.Turnout < 0.827 || r.Turnout > 0.828 { // 1490/1800
		t.Errorf("turnout = %v, want ~0.8278", r.Turnout)
	}
	if len(r.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2", len(r.Candidates))
	}
	if r.Candidates[0].Party != "A당" || r.Candidates[0].Votes != 800 { // 500+200+100
		t.Errorf("candidate[0] = %+v, want A당 800", r.Candidates[0])
	}
	if s := r.Candidates[0].Share; s < 0.547 || s > 0.549 { // 800/1460
		t.Errorf("share = %v, want ~0.5479", s)
	}
}

func TestAggregateByVoteType(t *testing.T) {
	out := Aggregate(sampleRecs(), AggSgg, true)
	if len(out) != 2 {
		t.Fatalf("got %d groups, want 2 (본투표 + 관내사전)", len(out))
	}
	byType := map[string]AggregatedRecord{}
	for _, r := range out {
		byType[r.VoteType] = r
	}
	if byType["본투표"].Votes != 1200 { // 800+400
		t.Errorf("본투표 votes = %d, want 1200", byType["본투표"].Votes)
	}
	if byType["관내사전"].Votes != 290 {
		t.Errorf("관내사전 votes = %d, want 290", byType["관내사전"].Votes)
	}
}

func TestAggregateSidoDropsCandidates(t *testing.T) {
	out := Aggregate(sampleRecs(), AggSido, false)
	if len(out) != 1 {
		t.Fatalf("got %d groups, want 1 시도", len(out))
	}
	r := out[0]
	if r.Sido != "서울" || r.District != "" {
		t.Errorf("sido dims wrong: %+v", r)
	}
	if len(r.Candidates) != 0 {
		t.Errorf("sido level must drop candidates, got %d", len(r.Candidates))
	}
	if r.Votes != 1490 {
		t.Errorf("metrics still summed: votes = %d, want 1490", r.Votes)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/nec -run TestAggregate -v`
Expected: FAIL — `undefined: Aggregate` (그리고 AggSgg 등)

- [ ] **Step 3: Create aggregate.go**

`internal/nec/aggregate.go`:

```go
package nec

// AggLevel selects the grouping granularity for Aggregate.
type AggLevel string

const (
	AggNone     AggLevel = "none"
	AggTown     AggLevel = "town"
	AggSgg      AggLevel = "sgg"
	AggSido     AggLevel = "sido"
	AggNational AggLevel = "national"
)

// CandidateShare is a candidate's summed votes plus the share derived from the
// group's valid votes (정의: 후보득표 / 유효투표수).
type CandidateShare struct {
	Party string  `json:"party"`
	Name  string  `json:"name"`
	Votes int     `json:"votes"`
	Share float64 `json:"share"`
}

// AggregatedRecord is one grouping of polling units rolled up to AggLevel, with
// neutral derived parameters. Candidates are summed only where comparable
// (town/sgg); at sido/national the slice is empty because districts differ.
type AggregatedRecord struct {
	Level      string           `json:"level"`
	Sido       string           `json:"sido,omitempty"`
	District   string           `json:"district,omitempty"`
	Town       string           `json:"town,omitempty"`
	VoteType   string           `json:"voteType,omitempty"`
	Electorate int              `json:"electorate"`
	Votes      int              `json:"votes"`
	Invalid    int              `json:"invalid"`
	ValidVotes int              `json:"validVotes"`
	Abstention int              `json:"abstention"`
	Turnout    float64          `json:"turnout"`
	Candidates []CandidateShare `json:"candidates,omitempty"`
}

// Aggregate rolls polling-unit records up to the requested level, optionally
// splitting each group by vote type. Group and candidate order follow first
// appearance for deterministic output. All derived values are reproducible
// arithmetic — no judgment is applied.
func Aggregate(recs []ResultRecord, level AggLevel, byVoteType bool) []AggregatedRecord {
	keepCands := level == AggTown || level == AggSgg

	type group struct {
		rec     AggregatedRecord
		candIdx map[string]int
	}
	order := []string{}
	groups := map[string]*group{}

	for _, r := range recs {
		key := groupKey(r, level, byVoteType)
		g, ok := groups[key]
		if !ok {
			g = &group{rec: newAgg(r, level, byVoteType), candIdx: map[string]int{}}
			groups[key] = g
			order = append(order, key)
		}
		g.rec.Electorate += r.Electorate
		g.rec.Votes += r.Votes
		g.rec.Invalid += r.Invalid
		g.rec.Abstention += r.Abstention
		if keepCands {
			for _, c := range r.Candidates {
				ck := c.Party + "\x1f" + c.Name
				if idx, ok := g.candIdx[ck]; ok {
					g.rec.Candidates[idx].Votes += c.Votes
				} else {
					g.candIdx[ck] = len(g.rec.Candidates)
					g.rec.Candidates = append(g.rec.Candidates, CandidateShare{Party: c.Party, Name: c.Name, Votes: c.Votes})
				}
			}
		}
	}

	out := make([]AggregatedRecord, 0, len(order))
	for _, key := range order {
		rec := groups[key].rec
		rec.ValidVotes = rec.Votes - rec.Invalid
		if rec.Electorate > 0 {
			rec.Turnout = float64(rec.Votes) / float64(rec.Electorate)
		}
		for i := range rec.Candidates {
			if rec.ValidVotes > 0 {
				rec.Candidates[i].Share = float64(rec.Candidates[i].Votes) / float64(rec.ValidVotes)
			}
		}
		out = append(out, rec)
	}
	return out
}

func groupKey(r ResultRecord, level AggLevel, byVoteType bool) string {
	var k string
	switch level {
	case AggTown:
		k = r.Sido + "\x1f" + r.District + "\x1f" + r.Town
	case AggSgg:
		k = r.Sido + "\x1f" + r.District
	case AggSido:
		k = r.Sido
	case AggNational:
		k = "전국"
	}
	if byVoteType {
		k += "\x1f" + r.VoteType
	}
	return k
}

func newAgg(r ResultRecord, level AggLevel, byVoteType bool) AggregatedRecord {
	rec := AggregatedRecord{Level: string(level)}
	switch level {
	case AggTown:
		rec.Sido, rec.District, rec.Town = r.Sido, r.District, r.Town
	case AggSgg:
		rec.Sido, rec.District = r.Sido, r.District
	case AggSido:
		rec.Sido = r.Sido
	case AggNational:
		// no spatial dimension
	}
	if byVoteType {
		rec.VoteType = r.VoteType
	}
	return rec
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/nec -run TestAggregate -v`
Expected: PASS (3개 모두)

- [ ] **Step 5: Commit**

```bash
git add internal/nec/aggregate.go internal/nec/nec_test.go
git commit -m "feat(nec): 다단계 집계 + 표준 파생 파라미터(투표율·득표율·유효투표수)"
```

---

### Task 3: CLI 노출 — nec results --aggregate / --by-votetype

**Files:**
- Modify: `cmd/kvote/nec.go` (necResultsCmd 플래그·분기 + renderResults 표 보강 + renderAggregated 신설)
- Test: 수동 라이브 검증(가이드 포함)

**Interfaces:**
- Consumes: `nec.Aggregate`, `nec.AggLevel`/상수, `nec.AggregatedRecord` (Task 2); `nec.ResultRecord.VoteType` (Task 1).
- Produces: 사용자용 `nec results <pk> --aggregate {none|town|sgg|sido|national} [--by-votetype]`.

- [ ] **Step 1: Add flags and branch in necResultsCmd**

`cmd/kvote/nec.go` 의 `necResultsCmd` 에서 `var file string` 선언을 다음으로 교체:

```go
	var file, aggregate string
	var byVoteType bool
```

`recs, err := nec.ParseResults(raw)` 직후의 기존 블록:

```go
			recs, err := nec.ParseResults(raw)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "정규화 완료: %d개 투표구\n", len(recs))
			return renderResults(cmd, format, recs)
```

다음으로 교체:

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

그리고 플래그 등록부 `c.Flags().StringVar(&file, "file", ...)` 다음에 추가:

```go
	c.Flags().StringVar(&aggregate, "aggregate", "none", "집계 단위: none|town|sgg|sido|national")
	c.Flags().BoolVar(&byVoteType, "by-votetype", false, "투표유형(본/관내사전/관외사전/거소선상)으로 분리 (집계와 함께)")
```

- [ ] **Step 2: Add parseAggLevel and renderAggregated helpers**

`cmd/kvote/nec.go` 의 `renderResults` 함수 바로 위에 추가:

```go
func parseAggLevel(s string) (nec.AggLevel, bool) {
	switch nec.AggLevel(s) {
	case nec.AggNone, nec.AggTown, nec.AggSgg, nec.AggSido, nec.AggNational:
		return nec.AggLevel(s), true
	}
	return "", false
}

func renderAggregated(cmd *cobra.Command, format output.Format, recs []nec.AggregatedRecord) error {
	switch format {
	case output.JSON:
		return output.WriteJSON(cmd.OutOrStdout(), recs)
	case output.JSONL:
		items := make([]any, len(recs))
		for i := range recs {
			items[i] = recs[i]
		}
		return output.WriteJSONL(cmd.OutOrStdout(), items)
	default:
		headers := []string{"level", "시도", "선거구", "읍면동", "투표유형", "선거인수", "투표수", "투표율", "후보수"}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			rows = append(rows, []string{
				r.Level, r.Sido, r.District, r.Town, r.VoteType,
				fmt.Sprint(r.Electorate), fmt.Sprint(r.Votes),
				fmt.Sprintf("%.1f%%", r.Turnout*100), fmt.Sprint(len(r.Candidates)),
			})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}
```

- [ ] **Step 3: Add 투표유형 column to renderResults table**

`cmd/kvote/nec.go` 의 `renderResults` 안 `default:` 분기에서 헤더·행을 수정 — 기존:

```go
		headers := []string{"시도", "선거구", "읍면동", "투표구", "선거인수", "투표수", "무효", "기권", "후보수"}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			rows = append(rows, []string{
				r.Sido, r.District, r.Town, r.Booth,
				fmt.Sprint(r.Electorate), fmt.Sprint(r.Votes), fmt.Sprint(r.Invalid),
				fmt.Sprint(r.Abstention), fmt.Sprint(len(r.Candidates)),
			})
		}
```

다음으로 교체:

```go
		headers := []string{"시도", "선거구", "읍면동", "투표구", "투표유형", "선거인수", "투표수", "무효", "기권", "후보수"}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			rows = append(rows, []string{
				r.Sido, r.District, r.Town, r.Booth, r.VoteType,
				fmt.Sprint(r.Electorate), fmt.Sprint(r.Votes), fmt.Sprint(r.Invalid),
				fmt.Sprint(r.Abstention), fmt.Sprint(len(r.Candidates)),
			})
		}
```

- [ ] **Step 4: Build and run full tests**

Run: `make build && go test ./... 2>&1 | grep -E 'ok|FAIL'`
Expected: build 성공, 모든 패키지 `ok`

- [ ] **Step 5: Live verification (이미 받은 CSV로)**

Run:
```bash
CSV=$(ls /tmp/necpull/*.csv | head -1)
./bin/kvote -f jsonl nec results --file "$CSV" --aggregate sgg --by-votetype 2>/dev/null | \
  python3 -c "import sys,json; rows=[json.loads(l) for l in sys.stdin if l.strip()]; \
  print('그룹:',len(rows)); \
  [print(f\"  {r['sido']} {r['district']} [{r['voteType']}] 투표율 {r['turnout']*100:.1f}% 1위 {r['candidates'][0]['party']} {r['candidates'][0]['name']} {r['candidates'][0]['share']*100:.1f}%\") for r in rows[:4]]"
```
Expected: (선거구 × 투표유형) 그룹들이 나오고, 각 그룹에 투표율·후보 득표율(share) 표시. 같은 선거구의 본투표/관내사전/관외사전 그룹이 나란히 → 소비자가 직접 비교 가능(우리는 비교/판단하지 않음).

- [ ] **Step 6: Commit**

```bash
git add cmd/kvote/nec.go
git commit -m "feat(nec): nec results --aggregate/--by-votetype (집계 뷰 노출)"
```

---

### Task 4: 문서 — README 사용 예시 + CLAUDE.md 아키텍처

**Files:**
- Modify: `README.md` (NEC Quick Start 예시)
- Modify: `CLAUDE.md` (internal/nec 아키텍처 줄에 aggregate.go)

**Interfaces:** Consumes: Task 1–3 의 최종 플래그/파일.

- [ ] **Step 1: README 예시 추가**

`README.md` 의 NEC 예시 블록에서 `kvote nec results --file ...` 줄 다음에 추가:

```bash
# 집계 뷰 (중립 파라미터 — 비교·판단은 소비자/AI 에이전트가)
kvote nec results 15025527 --aggregate sgg --by-votetype -f jsonl   # 선거구×투표유형
kvote nec results 15025527 --aggregate sido -f table                # 시도별 투표율
```

- [ ] **Step 2: CLAUDE.md 아키텍처 갱신**

`CLAUDE.md` 의 `internal/nec/` 블록에서 `results.go` 줄 다음에 추가:

```
  aggregate.go      투표구 레코드 → 다단계 집계(AggLevel) + 파생값(투표율·득표율·유효표). 중립.
```

- [ ] **Step 3: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs(nec): 집계 플래그 사용 예시 + aggregate.go 아키텍처"
```

---

## Self-Review

**Spec coverage:**
- 투표유형 차원 → Task 1 ✓
- 다단계 집계(town/sgg/sido/national) + by-votetype → Task 2 ✓
- 파생값(유효투표수·투표율·득표율, 정의 명시) → Task 2 (Global Constraints에 정의 고정) ✓
- 후보 합산은 sgg 이하만, sido↑ 비움 → Task 2 `keepCands` + TestAggregateSidoDropsCandidates ✓
- 원자료 보존 → ResultRecord 원필드 유지, AggregatedRecord에 원지표 모두 포함 ✓
- CLI 플래그 + none no-op → Task 3 (level==none 분기) ✓
- 비범위(verify/플래그/이상치 없음) → 계획에 그런 작업 없음 ✓
- 문서 → Task 4 ✓

**Placeholder scan:** 모든 step에 실제 코드/명령 포함. 없음.

**Type consistency:** `ResultRecord.VoteType`(T1) → `Aggregate`/`groupKey`/`newAgg`가 사용(T2) → CLI `parseAggLevel`/`renderAggregated`가 `nec.AggLevel`·`nec.AggregatedRecord` 사용(T3). 상수명(AggNone/Town/Sgg/Sido/National), 필드명(ValidVotes/Turnout/Share) 전 태스크 일치 ✓.
