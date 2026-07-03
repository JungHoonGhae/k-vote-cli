# nec turnout-analysis (성별·연령대별 투표율) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** data.go.kr의 "투표율 분석" ZIP(성별×연령대×지역 교차표)을 API 키 없이 정규화하는 `nec turnout-analysis` 명령과, 이를 로컬 SQLite에 적재하는 `db ingest turnout` + MCP `ingest_turnout` 를 만든다.

**Architecture:** 새 앵커 기반 파서 `internal/nec/turnout.go`(ZIP→wide→long 피벗)를 추가하고, 다운로드는 기존 `(*nec.Client).Download`(zip 그대로 반환)를 재사용한다. store/mcpserver/cmd는 v0.2.0의 IngestResults·ingest_results·db ingest 패턴을 그대로 확장한다.

**Tech Stack:** Go 1.26, `github.com/xuri/excelize/v2`(이미 사용 중), `archive/zip`(표준), `modernc.org/sqlite`, `modelcontextprotocol/go-sdk` v1.6.1, cobra.

## Global Constraints

- 모듈 경로: `github.com/JungHoonGhae/k-vote-cli`.
- **중립(타협 불가)**: 원자료 verbatim 보존. 소스가 제공한 투표율 `Rate`는 원자료로 저장(우리 파생 아님). 우리가 더하는 파생값은 정의 명시된 표준 변환 `rate_computed = voters/electorate*100`(electorate 0 → NULL) 하나뿐이며, 소스값(`rate_reported`)과 별개 컬럼. 플래그·점수·해석 없음.
- **키리스**: `nec.Client.Download`(키 없는 파일 다운로드)만 사용.
- **cgo 금지**: 새 `import "C"` 없음.
- **테스트 네트워크 없음**: 파서는 in-code로 만든 최소 xlsx→zip 픽스처, store는 `:memory:`/temp DB, mcpserver는 zip을 서빙하는 `httptest.Server`+`WithBaseURL`+in-memory transport.
- **schemaVersion 유지(=1, 올리지 않는다)**: turnout 테이블/뷰는 순수 추가 DDL이고 전부 `CREATE ... IF NOT EXISTS`. `migrate()`가 매 Open마다 전체 `SchemaSQL`을 재적용하므로 기존 v1 DB가 자동으로 turnout 객체를 얻는다(데이터 보존). 버전을 올리면 `migrate()`의 `v != 0 && v != schemaVersion` 가드가 기존 DB를 재생성 대상으로 오판하므로 올리지 않는다.
- 재사용 기존 심볼(변경 없이 소비):
  - `nec.atoiLoose(s string) int` (콤마 제거 정수 파싱, results.go)
  - `(*nec.Client).Download(ctx, pk, destDir) (string, error)` — 파일(zip 포함) 저장, 경로 반환
  - `store.DatasetMeta{ Source, PublicDataPk, Name, ElectionName string }`
  - store `IngestResults`/`IngestPolls` 패턴(트랜잭션·DELETE-then-INSERT 멱등)
  - mcpserver `errResult(msg string) *mcp.CallToolResult`, `ingestSummary{DatasetID int64; Rows int; Message string}`, `Deps{DBPath string; NEC *nec.Client; NESDC *nesdc.Client}`
  - cmd `newNECClient()`, `resolveDBPath()`, `resolveFormat()`, `output.WriteJSON/WriteJSONL/WriteTable`

## Turnout XLSX 레이아웃 (파서가 상대할 실제 구조)

- ZIP 내 대상 파일: 경로/파일명에 `성별·연령대별` 포함, 시트 = 시도별(17개).
- 각 시트:
  - **row0**: 제목. `성별·연령대별 투표율(구시군별)` 또는 `(선거구별)` 포함 → RegionLevel.
  - **대괄호 마커 행**(col0가 `[`로 시작, 예 `[표본-일반][서울특별시]`): 첫 `[...]` 안 = Category(verbatim).
  - **`구분` 헤더 행**(col0 == "구분"): col3부터 연령대 라벨(`합계`,`18세`,`19세`,`20-24세`,…,`80세이상`). 열 인덱스 확보.
  - **데이터 블록**: col0 = 지역명(`전체`=시도전체, 이후 각 구시군/선거구; 빈 셀이면 직전 지역 유지), col1 = 성별(`합계`/`남자`/`여자`), col2 = 지표(`선거인수`/`투표자수`/`투표율`). 지역당 9행(3성별×3지표).
- **wide→long 피벗**: (지역,성별)마다 선거인수·투표자수·투표율 세 행을 모아, 연령대 열 각각에 대해 TurnoutRecord 1건 emit.
- 값은 excelize에서 문자열(콤마 포함 가능). 정수는 `atoiLoose`, 투표율은 `strconv.ParseFloat`(빈/`-`은 0).

---

## Task 1: TurnoutRecord + ParseTurnoutAnalysis 파서

**Files:**
- Create: `internal/nec/turnout.go`
- Create: `internal/nec/turnout_test.go`

**Interfaces:**
- Produces:
  - `type TurnoutRecord struct { Election, Category, RegionLevel, Sido, Region, Gender, AgeGroup string; Electorate, Voters int; Rate float64 }` (json 태그: election/category/regionLevel/sido/region/gender/ageGroup/electorate/voters/rate)
  - `func ParseTurnoutAnalysis(zipRaw []byte) ([]TurnoutRecord, error)` — Election 필드는 빈 문자열(호출부가 채움).

- [ ] **Step 1: 실패하는 테스트 작성**

Create `internal/nec/turnout_test.go`. 테스트는 excelize로 최소 xlsx를 만들어 zip으로 묶는 헬퍼로 픽스처를 구성한다(바이너리 커밋 안 함, 네트워크 없음):

```go
package nec

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildTurnoutZip makes a minimal 성별·연령대별 투표율 xlsx and wraps it in a zip,
// matching the real layout: title row, bracket marker, 구분 header, then
// per-region blocks of (전체/남자/여자) × (선거인수/투표자수/투표율).
func buildTurnoutZip(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sh := "서울"
	f.SetSheetName("Sheet1", sh)
	set := func(cell, v string) { f.SetCellValue(sh, cell, v) }
	// row1 title, row3 marker, row4 구분 header, rows5.. data
	set("A1", "성별·연령대별 투표율(구시군별)")
	set("A3", "[표본-일반][서울특별시]")
	set("A4", "구분")
	set("D4", "합계")
	set("E4", "18세")
	set("F4", "20-24세")
	// 전체 지역, 합계 성별
	set("A5", "전체")
	set("B5", "합계")
	set("C5", "선거인수")
	set("D5", "710,801")
	set("E5", "5,910")
	set("F5", "46,137")
	set("C6", "투표자수")
	set("D6", "493,617")
	set("E6", "3,673")
	set("F6", "26,835")
	set("C7", "투표율")
	set("D7", "69.4")
	set("E7", "62.1")
	set("F7", "58.2")
	// 전체 지역, 남자
	set("B8", "남자")
	set("C8", "선거인수")
	set("D8", "339,845")
	set("C9", "투표자수")
	set("D9", "233,296")
	set("C10", "투표율")
	set("D10", "68.6")
	// 전체 지역, 여자
	set("B11", "여자")
	set("C11", "선거인수")
	set("D11", "370,956")
	set("C12", "투표자수")
	set("D12", "260,321")
	set("C13", "투표율")
	set("D13", "70.2")
	// 다음 지역 종로구, 합계만 (블록 반복 검증용)
	set("A14", "종로구")
	set("B14", "합계")
	set("C14", "선거인수")
	set("D14", "100")
	set("C15", "투표자수")
	set("D15", "60")
	set("C16", "투표율")
	set("D16", "60.0")

	var xlsxBuf bytes.Buffer
	if err := f.Write(&xlsxBuf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("02_선거일 투표/02_성별·연령대별 투표율(구시군별).xlsx")
	w.Write(xlsxBuf.Bytes())
	// 매칭 안 되는 파일도 하나 넣어 graceful-skip 검증
	w2, _ := zw.Create("03_사전투표/01_전체.xlsx")
	w2.Write([]byte("not an xlsx"))
	zw.Close()
	return zipBuf.Bytes()
}

func TestParseTurnoutAnalysis(t *testing.T) {
	recs, err := ParseTurnoutAnalysis(buildTurnoutZip(t))
	if err != nil {
		t.Fatalf("ParseTurnoutAnalysis: %v", err)
	}
	// 전체(3성별×3연령) + 종로구(1성별×3연령... 합계만) = 9 + 3 = 12
	if len(recs) != 12 {
		t.Fatalf("got %d records, want 12", len(recs))
	}
	// 전체·합계·합계(연령) 레코드 확인
	var got *TurnoutRecord
	for i := range recs {
		r := &recs[i]
		if r.Region == "전체" && r.Gender == "합계" && r.AgeGroup == "합계" {
			got = r
			break
		}
	}
	if got == nil {
		t.Fatal("전체/합계/합계 레코드 없음")
	}
	if got.RegionLevel != "구시군" {
		t.Errorf("RegionLevel = %q, want 구시군", got.RegionLevel)
	}
	if got.Category != "표본-일반" {
		t.Errorf("Category = %q, want 표본-일반", got.Category)
	}
	if got.Sido != "서울" {
		t.Errorf("Sido = %q", got.Sido)
	}
	if got.Electorate != 710801 || got.Voters != 493617 {
		t.Errorf("electorate/voters = %d/%d, want 710801/493617", got.Electorate, got.Voters)
	}
	if got.Rate != 69.4 {
		t.Errorf("Rate = %v, want 69.4", got.Rate)
	}
	if got.Election != "" {
		t.Errorf("Election should be empty (caller fills), got %q", got.Election)
	}
}

func TestParseTurnoutAnalysisNoTarget(t *testing.T) {
	// zip with only a non-matching file → error
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("03_사전투표/01_전체.xlsx")
	w.Write([]byte("nope"))
	zw.Close()
	if _, err := ParseTurnoutAnalysis(zipBuf.Bytes()); err == nil {
		t.Fatal("expected error when no target sheet matched")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/nec/ -run TestParseTurnoutAnalysis -v`
Expected: FAIL — `undefined: ParseTurnoutAnalysis` / `TurnoutRecord`.

- [ ] **Step 3: 파서 구현**

Create `internal/nec/turnout.go`:

```go
package nec

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// TurnoutRecord is one normalized (지역 × 성별 × 연령대) turnout cell from a NEC
// "투표율 분석" dataset. Rate is the source-reported value (원자료), not derived.
type TurnoutRecord struct {
	Election    string  `json:"election"`
	Category    string  `json:"category"`
	RegionLevel string  `json:"regionLevel"`
	Sido        string  `json:"sido"`
	Region      string  `json:"region"`
	Gender      string  `json:"gender"`
	AgeGroup    string  `json:"ageGroup"`
	Electorate  int     `json:"electorate"`
	Voters      int     `json:"voters"`
	Rate        float64 `json:"rate"`
}

// ParseTurnoutAnalysis unzips a NEC 투표율 분석 dataset and parses every sheet that
// matches the 성별·연령대별 투표율 cross-tab anchor into long-format records. Files
// and sheets that don't match are skipped with a stderr warning (like ParseResultsXLSX).
// Election is left empty; the caller fills it from the dataset name.
func ParseTurnoutAnalysis(zipRaw []byte) ([]TurnoutRecord, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipRaw), int64(len(zipRaw)))
	if err != nil {
		return nil, fmt.Errorf("open zip (nec pull 로 원본 확인): %w", err)
	}
	var out []TurnoutRecord
	for _, zf := range zr.File {
		if !strings.HasSuffix(strings.ToLower(zf.Name), ".xlsx") {
			continue
		}
		if !strings.Contains(zf.Name, "성별") { // target filename hint
			continue
		}
		raw, err := readZipEntry(zf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %q: %v\n", zf.Name, err)
			continue
		}
		recs := parseTurnoutXLSX(zf.Name, raw)
		out = append(out, recs...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("성별·연령대별 투표율 시트를 찾지 못했습니다 (사전/재외/PDF 데이터셋일 수 있음 — nec pull 로 원본 확인)")
	}
	return out, nil
}

func readZipEntry(zf *zip.File) ([]byte, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// parseTurnoutXLSX parses one xlsx (multi-sheet by 시도) into records. Non-matching
// sheets are skipped with a warning; a malformed workbook yields no records.
func parseTurnoutXLSX(fname string, raw []byte) []TurnoutRecord {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "skip %q: open xlsx: %v\n", fname, err)
		return nil
	}
	defer f.Close()

	var out []TurnoutRecord
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil || len(rows) < 5 {
			continue
		}
		recs := parseTurnoutSheet(sheet, rows)
		out = append(out, recs...)
	}
	return out
}

// cellAt returns row[i] trimmed, or "" when out of range.
func cellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func norm(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), " ", "") }

// parseTurnoutSheet parses a single 시도 sheet. Returns nil when the sheet lacks
// the 성별·연령대별 anchor (구분 header + 선거인수/투표자수/투표율 rows).
func parseTurnoutSheet(sheet string, rows [][]string) []TurnoutRecord {
	regionLevel := ""
	category := ""
	headerRow := -1
	ageCols := []int{}      // column indices of age buckets
	ageLabels := []string{} // parallel labels

	for i, row := range rows {
		c0 := cellAt(row, 0)
		if strings.Contains(c0, "구시군별") {
			regionLevel = "구시군"
		} else if strings.Contains(c0, "선거구별") {
			regionLevel = "선거구"
		}
		if strings.HasPrefix(c0, "[") {
			if end := strings.Index(c0, "]"); end > 1 {
				category = c0[1:end]
			}
		}
		if norm(c0) == "구분" {
			headerRow = i
			for j := 3; j < len(row); j++ {
				lbl := cellAt(row, j)
				if lbl == "" {
					continue
				}
				ageCols = append(ageCols, j)
				ageLabels = append(ageLabels, lbl)
			}
			break
		}
	}
	if headerRow < 0 || len(ageCols) == 0 {
		return nil // not a target sheet
	}

	var out []TurnoutRecord
	curRegion := ""
	// walk data rows in groups: col0 region (sticky), col1 gender, col2 metric.
	// A (region,gender) block is 3 consecutive metric rows: 선거인수/투표자수/투표율.
	i := headerRow + 1
	for i < len(rows) {
		row := rows[i]
		if r := cellAt(row, 0); r != "" {
			curRegion = r
		}
		gender := cellAt(row, 1)
		metric := norm(cellAt(row, 2))
		if curRegion == "" || gender == "" || metric != "선거인수" {
			i++
			continue
		}
		// Expect this row = 선거인수, next = 투표자수, next+1 = 투표율.
		elecRow := rows[i]
		var voteRow, rateRow []string
		if i+1 < len(rows) {
			voteRow = rows[i+1]
		}
		if i+2 < len(rows) {
			rateRow = rows[i+2]
		}
		if norm(cellAt(voteRow, 2)) != "투표자수" || norm(cellAt(rateRow, 2)) != "투표율" {
			i++
			continue
		}
		for k, col := range ageCols {
			out = append(out, TurnoutRecord{
				Category:    category,
				RegionLevel: regionLevel,
				Sido:        sheet,
				Region:      curRegion,
				Gender:      gender,
				AgeGroup:    ageLabels[k],
				Electorate:  atoiLoose(cellAt(elecRow, col)),
				Voters:      atoiLoose(cellAt(voteRow, col)),
				Rate:        parseRate(cellAt(rateRow, col)),
			})
		}
		i += 3
	}
	return out
}

// parseRate parses a percentage like "69.4"; blank or "-" yields 0.
func parseRate(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" || s == "-" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/nec/ -run TestParseTurnoutAnalysis -v`
Expected: PASS (both tests).

- [ ] **Step 5: 커밋**

```bash
git add internal/nec/turnout.go internal/nec/turnout_test.go
git commit -m "feat(nec): 투표율 분석 ZIP → 성별·연령대별 TurnoutRecord 파서 (앵커 기반)"
```

---

## Task 2: turnout 테이블·뷰 + IngestTurnout

**Files:**
- Modify: `internal/store/schema.go`
- Modify: `internal/store/ingest.go`
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `nec.TurnoutRecord` (Task 1), `store.DatasetMeta`, `store.Open`.
- Produces: `(*store.DB).IngestTurnout(meta DatasetMeta, recs []nec.TurnoutRecord) (int64, error)`; DB objects `turnout` table + `v_turnout_derived` view.

- [ ] **Step 1: 실패하는 테스트 작성**

Add to `internal/store/store_test.go`:

```go
func sampleTurnout() []nec.TurnoutRecord {
	return []nec.TurnoutRecord{
		{Election: "제22대 총선", Category: "표본-일반", RegionLevel: "구시군", Sido: "서울",
			Region: "전체", Gender: "합계", AgeGroup: "합계", Electorate: 710801, Voters: 493617, Rate: 69.4},
		{Election: "제22대 총선", Category: "표본-일반", RegionLevel: "구시군", Sido: "서울",
			Region: "전체", Gender: "남자", AgeGroup: "18세", Electorate: 0, Voters: 0, Rate: 0},
	}
}

func TestIngestTurnoutIdempotent(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "k.db"))
	defer db.Close()
	meta := DatasetMeta{Source: "nec", PublicDataPk: "15143936", Name: "투표율분석.zip"}
	if _, err := db.IngestTurnout(meta, sampleTurnout()); err != nil {
		t.Fatalf("ingest 1: %v", err)
	}
	if _, err := db.IngestTurnout(meta, sampleTurnout()); err != nil {
		t.Fatalf("ingest 2: %v", err)
	}
	var n int
	db.SQL().QueryRow("SELECT count(*) FROM turnout").Scan(&n)
	if n != 2 {
		t.Errorf("재적재 후 turnout=%d, want 2 (중복 없음)", n)
	}
	// v_turnout_derived: electorate>0 → rate_computed 계산, electorate=0 → NULL
	var rc *float64
	db.SQL().QueryRow(`SELECT rate_computed FROM v_turnout_derived
		WHERE region='전체' AND gender='합계' AND age_group='합계'`).Scan(&rc)
	if rc == nil {
		t.Fatal("rate_computed nil for electorate>0")
	}
	want := float64(493617) / 710801 * 100
	if *rc < want-1e-6 || *rc > want+1e-6 {
		t.Errorf("rate_computed = %v, want %v", *rc, want)
	}
	var rc0 *float64
	db.SQL().QueryRow(`SELECT rate_computed FROM v_turnout_derived WHERE age_group='18세'`).Scan(&rc0)
	if rc0 != nil {
		t.Errorf("rate_computed should be NULL for electorate=0, got %v", *rc0)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/store/ -run TestIngestTurnout -v`
Expected: FAIL — `undefined: IngestTurnout` (그리고 turnout 테이블 없음).

- [ ] **Step 3: 스키마에 turnout 추가**

In `internal/store/schema.go`, inside the `SchemaSQL` string, append BEFORE the closing backtick (after the `v_agg_national` view):

```sql

CREATE TABLE IF NOT EXISTS turnout (
  id           INTEGER PRIMARY KEY,
  dataset_id   INTEGER NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
  election     TEXT, category TEXT, region_level TEXT, sido TEXT, region TEXT,
  gender       TEXT, age_group TEXT,
  electorate   INTEGER, voters INTEGER, rate REAL   -- rate = 소스 제공 원자료
);
CREATE INDEX IF NOT EXISTS idx_turnout_dataset ON turnout(dataset_id);

-- 표준 파생: 소스 보고 투표율(rate_reported) 옆에 재계산값을 나란히. 정의 명시(중립).
CREATE VIEW IF NOT EXISTS v_turnout_derived AS
SELECT id, dataset_id, election, category, region_level, sido, region, gender, age_group,
       electorate, voters, rate AS rate_reported,
       CASE WHEN electorate > 0 THEN CAST(voters AS REAL)/electorate*100 END AS rate_computed
FROM turnout;
```

Also add to `SchemaDoc` (before its closing backtick), one block:

```
- turnout(id, dataset_id, election, category[표본/표본-일반…], region_level[구시군|선거구],
    sido, region, gender[합계|남자|여자], age_group[합계|18세|20-24세…], electorate, voters,
    rate)  -- rate 는 소스가 보고한 투표율 원자료
- v_turnout_derived: 위 + rate_computed = voters/electorate*100 (electorate 0 → NULL).
    rate_reported(소스값) 와 rate_computed(재계산) 를 나란히 — 판단은 소비자.
```

- [ ] **Step 4: IngestTurnout 구현**

In `internal/store/ingest.go`, append:

```go
// IngestTurnout replaces any existing dataset with the same (source, public_data_pk)
// and inserts turnout cross-tab records verbatim. All-or-nothing (transaction).
func (d *DB) IngestTurnout(meta DatasetMeta, recs []nec.TurnoutRecord) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM datasets WHERE source = ? AND public_data_pk = ?`,
		meta.Source, meta.PublicDataPk); err != nil {
		return 0, err
	}
	res, err := tx.Exec(
		`INSERT INTO datasets(source, public_data_pk, name, election_name, ingested_at, row_count)
		 VALUES(?,?,?,?,?,?)`,
		meta.Source, meta.PublicDataPk, meta.Name, meta.ElectionName,
		time.Now().UTC().Format(time.RFC3339), len(recs))
	if err != nil {
		return 0, err
	}
	dsID, _ := res.LastInsertId()

	for _, r := range recs {
		if _, err := tx.Exec(
			`INSERT INTO turnout(dataset_id, election, category, region_level, sido, region,
			   gender, age_group, electorate, voters, rate) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			dsID, r.Election, r.Category, r.RegionLevel, r.Sido, r.Region,
			r.Gender, r.AgeGroup, r.Electorate, r.Voters, r.Rate); err != nil {
			return 0, fmt.Errorf("insert turnout: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return dsID, nil
}
```

(`nec` and `time` are already imported in ingest.go.)

- [ ] **Step 5: 테스트 통과 확인**

Run: `go test ./internal/store/ -run 'TestIngestTurnout|TestMigrate' -v`
Expected: PASS.

- [ ] **Step 6: 커밋**

```bash
git add internal/store/schema.go internal/store/ingest.go internal/store/store_test.go
git commit -m "feat(store): turnout 테이블·v_turnout_derived 뷰 + IngestTurnout(멱등)"
```

---

## Task 3: MCP ingest_turnout tool

**Files:**
- Modify: `internal/mcpserver/ingest.go`
- Modify: `internal/mcpserver/server_test.go`

**Interfaces:**
- Consumes: `deps.NEC.Download`, `nec.ParseTurnoutAnalysis`, `store.Open`/`IngestTurnout`, existing `errResult`/`ingestSummary`.
- Produces: `ingest_turnout` tool registered inside `registerIngestTools`.

- [ ] **Step 1: 실패하는 테스트 작성**

Add to `internal/mcpserver/server_test.go` a round-trip test. Reuse the zip-fixture idea from Task 1 but serve it over the nec download 3-step endpoints (same pattern as `necFixtureServer` already in this file — read it and mirror it, swapping the served bytes for a turnout zip built via `buildTurnoutZipBytes` below and the download filename ending in `.zip`).

```go
// buildTurnoutZipBytes mirrors internal/nec/turnout_test.go's fixture: a minimal
// 성별·연령대별 투표율 xlsx wrapped in a zip. Kept local to avoid cross-package test deps.
func buildTurnoutZipBytes(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sh := "서울"
	f.SetSheetName("Sheet1", sh)
	s := func(c, v string) { f.SetCellValue(sh, c, v) }
	s("A1", "성별·연령대별 투표율(구시군별)")
	s("A3", "[표본-일반][서울특별시]")
	s("A4", "구분")
	s("D4", "합계")
	s("A5", "전체")
	s("B5", "합계")
	s("C5", "선거인수")
	s("D5", "100")
	s("C6", "투표자수")
	s("D6", "60")
	s("C7", "투표율")
	s("D7", "60.0")
	var xb bytes.Buffer
	f.Write(&xb)
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("02_선거일 투표/성별·연령대별.xlsx")
	w.Write(xb.Bytes())
	zw.Close()
	return zb.Bytes()
}

func TestIngestTurnoutTool(t *testing.T) {
	zipBytes := buildTurnoutZipBytes(t)
	const detailHTML = `<html><body><script>
function init(){ fn_fileDataDown('15143936', 'uddi:aaaa-bbbb-cc', '','1', '3'); }
</script></body></html>`
	const resolveJSON = `{"status":true,"atchFileId":"FILE_0001","fileDetailSn":1,
"dataSetFileDetailInfo":{"dataNm":"투표율분석"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/15143936/fileData.do":
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.Write([]byte(detailHTML))
		case "/tcs/dss/selectFileDataDownload.do":
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			w.Write([]byte(resolveJSON))
		case "/cmm/cmm/fileDownload.do":
			w.Header().Set("Content-Disposition", `attachment; filename="투표율분석.zip"`)
			w.Header().Set("Content-Type", "application/zip")
			w.Write(zipBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := filepath.Join(t.TempDir(), "k.db")
	deps := Deps{DBPath: p, NEC: nec.New(nec.WithBaseURL(srv.URL), nec.WithDelay(0))}
	sv := New(deps)
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	sv.Connect(ctx, st, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, _ := client.Connect(ctx, ct, nil)
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "ingest_turnout", Arguments: map[string]any{"pk": "15143936"}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
	// verify rows landed
	ro, _ := store.OpenReadOnly(p)
	defer ro.Close()
	qr, err := ro.Query("SELECT count(*) FROM turnout", 10)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(qr.Rows) != 1 {
		t.Fatalf("no count row")
	}
}
```

Ensure the test file imports `archive/zip`, `bytes`, `github.com/xuri/excelize/v2`, and the existing mcp/nec/store imports (add any missing).

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/mcpserver/ -run TestIngestTurnoutTool -v`
Expected: FAIL — tool `ingest_turnout` unknown (or compile error until Step 3).

- [ ] **Step 3: tool 구현**

In `internal/mcpserver/ingest.go`, add an input type near the others:

```go
type ingestTurnoutIn struct {
	PK string `json:"pk" jsonschema:"data.go.kr publicDataPk of the 투표율 분석 (ZIP) file dataset to download and ingest"`
}
```

and inside `registerIngestTools`, after the existing tools, register:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ingest_turnout",
		Description: "data.go.kr publicDataPk 의 투표율 분석(ZIP)을 API 키 없이 내려받아 성별·연령대별 투표율로 정규화 후 로컬 DB에 적재한다(멱등). 사전/재외·PDF 데이터셋은 미지원.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ingestTurnoutIn) (*mcp.CallToolResult, *ingestSummary, error) {
		dir, err := os.MkdirTemp("", "kvote-mcp-turnout-")
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		defer os.RemoveAll(dir)
		path, err := deps.NEC.Download(ctx, in.PK, dir)
		if err != nil {
			return errResult(fmt.Sprintf("다운로드 실패: %v", err)), nil, nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		recs, err := nec.ParseTurnoutAnalysis(raw)
		if err != nil {
			return errResult(fmt.Sprintf("정규화 실패: %v", err)), nil, nil
		}
		election := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		for i := range recs {
			recs[i].Election = election
		}
		db, err := store.Open(deps.DBPath)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		defer db.Close()
		dsID, err := db.IngestTurnout(store.DatasetMeta{Source: "nec", PublicDataPk: in.PK, Name: filepath.Base(path), ElectionName: election}, recs)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &ingestSummary{DatasetID: dsID, Rows: len(recs),
			Message: fmt.Sprintf("%d개 (지역×성별×연령) 셀 적재", len(recs))}, nil
	})
```

Add `"strings"` to ingest.go imports (filepath/os/fmt already present).

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/mcpserver/ -run 'TestIngestTurnoutTool|TestIngestResultsTool|TestQueryTool' -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/mcpserver/ingest.go internal/mcpserver/server_test.go
git commit -m "feat(mcpserver): ingest_turnout tool (투표율 분석 ZIP 키리스 수집→적재)"
```

---

## Task 4: CLI — `nec turnout-analysis` + `db ingest turnout`

**Files:**
- Modify: `cmd/kvote/nec.go`
- Modify: `cmd/kvote/db.go`

**Interfaces:**
- Consumes: `nec.ParseTurnoutAnalysis`, `newNECClient()`, `resolveDBPath()`, `resolveFormat()`, `store.Open`/`IngestTurnout`, `output.WriteJSON/WriteJSONL/WriteTable`.
- Produces: `nec turnout-analysis <pk> [--file ZIP]` command; `db ingest turnout <pk>` subcommand.

- [ ] **Step 1: `nec turnout-analysis` 명령 작성**

In `cmd/kvote/nec.go`, add a command builder and register it in the `nec` command group (find where sibling commands like `results` are added via `c.AddCommand(...)` and add `turnoutAnalysisCmd()`).

```go
func turnoutAnalysisCmd() *cobra.Command {
	var file string
	c := &cobra.Command{
		Use:   "turnout-analysis <publicDataPk>",
		Short: "투표율 분석(ZIP)을 성별·연령대별 투표율 레코드로 정규화",
		Long: `data.go.kr 의 "투표율 분석" 데이터셋(ZIP)을 받아 성별·연령대별·지역별 투표율을
정규화합니다. 개표결과에 없는 인구통계 축(누가 투표했는가)이라 독립적입니다.
소스가 보고한 투표율은 원자료로 보존합니다.

--file 로 이미 받은 ZIP 경로를 주면 다운로드 없이 파싱합니다.
(사전/재외투표·PDF 전용 데이터셋은 미지원 — nec pull 로 원본을 받으세요.)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			var raw []byte
			var election string
			switch {
			case file != "":
				if raw, err = os.ReadFile(file); err != nil {
					return err
				}
				election = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			case len(args) == 1:
				dir, err := os.MkdirTemp("", "kvote-turnout-")
				if err != nil {
					return err
				}
				defer os.RemoveAll(dir)
				path, err := newNECClient().Download(context.Background(), args[0], dir)
				if err != nil {
					return err
				}
				if raw, err = os.ReadFile(path); err != nil {
					return err
				}
				election = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			default:
				return fmt.Errorf("publicDataPk 또는 --file 중 하나가 필요합니다")
			}

			recs, err := nec.ParseTurnoutAnalysis(raw)
			if err != nil {
				return err
			}
			for i := range recs {
				recs[i].Election = election
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "정규화 완료: %d개 (지역×성별×연령) 셀\n", len(recs))
			return renderTurnout(cmd, format, recs)
		},
	}
	c.Flags().StringVar(&file, "file", "", "이미 받은 투표율 분석 ZIP 경로 (다운로드 생략)")
	return c
}

func renderTurnout(cmd *cobra.Command, format output.Format, recs []nec.TurnoutRecord) error {
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
		headers := []string{"시도", "지역", "성별", "연령대", "선거인수", "투표자수", "투표율", "구분"}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			rows = append(rows, []string{
				r.Sido, r.Region, r.Gender, r.AgeGroup,
				itoaTurnout(r.Electorate), itoaTurnout(r.Voters),
				strconv.FormatFloat(r.Rate, 'f', 1, 64), r.Category,
			})
		}
		return output.WriteTable(cmd.OutOrStdout(), headers, rows)
	}
}

func itoaTurnout(n int) string { return strconv.Itoa(n) }
```

Ensure `cmd/kvote/nec.go` imports include `strings`, `strconv`, `path/filepath` (add any missing). If `itoaTurnout`/an equivalent already exists in the file, reuse it instead of adding a duplicate.

- [ ] **Step 2: 빌드 + 명령 확인**

Run:
```bash
go build ./... && ./bin/kvote 2>/dev/null; make build && ./bin/kvote nec turnout-analysis --help
```
Expected: help prints; no build error. (`./bin/kvote` first line may error on no-args — ignore.)

- [ ] **Step 3: `db ingest turnout` 서브명령 작성**

In `cmd/kvote/db.go`'s `dbIngestCmd()`, add a `turnout` subcommand alongside `results`/`polls` (mirror the `results` subcommand exactly, swapping the parser + ingest call):

```go
	turnout := &cobra.Command{
		Use:   "turnout <publicDataPk>",
		Short: "투표율 분석 ZIP을 성별·연령대별로 적재 (멱등)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, err := resolveDBPath()
			if err != nil {
				return err
			}
			dir, err := os.MkdirTemp("", "kvote-db-turnout-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			path, err := newNECClient().Download(context.Background(), args[0], dir)
			if err != nil {
				return err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			recs, err := nec.ParseTurnoutAnalysis(raw)
			if err != nil {
				return err
			}
			election := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			for i := range recs {
				recs[i].Election = election
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			id, err := db.IngestTurnout(store.DatasetMeta{Source: "nec", PublicDataPk: args[0], Name: filepath.Base(path), ElectionName: election}, recs)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "적재 완료: dataset=%d, %d개 셀\n", id, len(recs))
			return nil
		},
	}
```

and add it to the `ingest.AddCommand(results, polls)` call → `ingest.AddCommand(results, polls, turnout)`. Add `"strings"` to db.go imports if missing (filepath/os/fmt/context/store/nec already present).

- [ ] **Step 4: 빌드 + 스모크 + 전체 테스트**

Run:
```bash
make build
./bin/kvote db ingest turnout --help
./bin/kvote db ingest --help
go build ./... && go vet ./... && go test ./...
```
Expected: help prints; all tests pass; vet clean.

- [ ] **Step 5: 커밋**

```bash
make fmt
git add cmd/kvote/nec.go cmd/kvote/db.go
git commit -m "feat(cmd): nec turnout-analysis + db ingest turnout (성별·연령대별 투표율)"
```

---

## Task 5: 라이브 검증 + 문서

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `CHANGELOG.md`

**Interfaces:** (문서 + 라이브 스모크)

- [ ] **Step 1: 라이브 스모크 (네트워크 — 실데이터로 end-to-end 확인)**

Run (네트워크 필요; 실패해도 코드 결함이 아니라 포털 이슈일 수 있으니 출력을 보고 판단):
```bash
./bin/kvote nec turnout-analysis 15143936 -f table 2>&1 | head -15
./bin/kvote db ingest turnout 15143936 2>&1 | tail -2
./bin/kvote db query "SELECT sido, count(*) c FROM turnout GROUP BY sido LIMIT 5" -f table 2>&1 | head -10
./bin/kvote db query "SELECT region, gender, age_group, rate_reported, round(rate_computed,1) FROM v_turnout_derived WHERE region='전체' AND gender='합계' LIMIT 5" -f table 2>&1 | head -10
```
Expected: 정규화 셀 출력, 적재 요약, 시도별 카운트, rate_reported≈rate_computed. (제22대 총선 pk=15143936.)

- [ ] **Step 2: CLAUDE.md 갱신**

`internal/nec/` 블록에 추가: `turnout.go  투표율 분석 ZIP → 성별·연령대별 TurnoutRecord (앵커 기반, graceful-skip)`.
`internal/store/` 블록의 schema/ingest 설명에 turnout 언급 추가.
`internal/mcpserver/ingest.go` 설명에 `ingest_turnout` 추가.
명령어 예시에 `./bin/kvote nec turnout-analysis <pk>` 추가.
핵심 설계 포인트에 한 줄: 투표율 분석은 개표결과에 없는 인구통계 축(성별·연령), `Rate`는 소스 원자료 보존 + `v_turnout_derived.rate_computed`는 정의 명시 재계산(중립).

- [ ] **Step 3: README.md 갱신**

기능 표/`nec` 섹션에 `nec turnout-analysis` 추가, MCP tool 표에 `ingest_turnout` 행 추가, `kvote db ingest` 예시에 `turnout` 추가. "성별·연령대별 투표율 → 여론조사 교차표와 같은 축으로 비교 가능" 한 줄로 가치 설명. API 키 없이·중립 톤 유지("비공식" 자기표기 금지).

- [ ] **Step 4: CHANGELOG.md 갱신**

`## [Unreleased]` 아래에 항목 추가:
```
### NEC — 투표율 (키리스)
- **`nec turnout-analysis`** — data.go.kr "투표율 분석" ZIP을 성별·연령대별·지역별 투표율로 정규화합니다. 개표결과에 없는 인구통계 축이라, 여론조사 교차표·개표결과와 나란히 비교할 수 있습니다.
- **`kvote db ingest turnout`** / MCP **`ingest_turnout`** — 위 데이터를 로컬 DB에 적재해 SQL로 교차 질의합니다. 소스 투표율은 원자료로 보존하고, `v_turnout_derived` 가 재계산값을 나란히 제공합니다.
```

- [ ] **Step 5: 커밋**

```bash
git add CLAUDE.md README.md CHANGELOG.md
git commit -m "docs: nec turnout-analysis·ingest_turnout 반영 + 라이브 검증"
```

---

## Self-Review (작성자 확인 결과)

**1. Spec coverage:**
- 파서(ZIP→wide→long, 앵커, graceful-skip) → Task 1 ✅
- TurnoutRecord 스키마(Rate=원자료, Election 호출부 채움) → Task 1 ✅ (Election 빈값 검증 포함)
- turnout 테이블 + v_turnout_derived(rate_computed, electorate 0 가드) → Task 2 ✅
- IngestTurnout 멱등·트랜잭션 → Task 2 ✅
- schemaVersion 유지(순수 추가 DDL) → Global Constraints + Task 2 IF NOT EXISTS ✅
- MCP ingest_turnout(키리스, errResult) → Task 3 ✅ (zip 픽스처 왕복)
- nec turnout-analysis + db ingest turnout → Task 4 ✅
- 라이브 검증 + 문서(CLAUDE/README/CHANGELOG) → Task 5 ✅
- 중립성(원자료 보존, 파생 1개 정의 명시) → 전 태스크 반영 ✅
- YAGNI(사전/재외·시간대별·분포/종합 제외) → 계획에 미포함 ✅

**2. Placeholder scan:** 실제 코드 전량 기재. 픽스처는 in-code 생성(바이너리 커밋 없음). Task 4의 "itoaTurnout 이미 있으면 재사용", Task 5 라이브 스모크의 네트워크 의존성은 명시적 조건부로 표기 — 플레이스홀더 아님.

**3. Type consistency:** `TurnoutRecord`(Task1) 필드가 store 컬럼(Task2)·CLI 렌더(Task4)·MCP(Task3)에서 일치. `DatasetMeta{Source:"nec", PublicDataPk:pk}` 키가 db.go·mcpserver에서 동일(멱등 정합). `ingestSummary`/`errResult`/`Deps` v0.2.0 심볼 재사용. `atoiLoose`(nec) 재사용, 신규 `parseRate`는 turnout.go 내부.

**주의(구현자):** go-sdk v1.6.1 API·`output.WriteTable(w, headers, rows)` 시그니처는 v0.2.0에서 검증됨 — 동일하게 사용. 컴파일 에러 시 메시지에 맞춰 import만 조정.
