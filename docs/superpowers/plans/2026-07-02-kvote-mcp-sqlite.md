# kvote mcp (SQLite 통합 데이터셋 + MCP 질의) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** AI 에이전트가 MCP 연결 하나로 한국 선거 공개 데이터의 탐색 → 키리스 수집 → SQL 질의를 끝낼 수 있는 `kvote mcp` 서버와 `internal/store` (SQLite) 계층을 만든다.

**Architecture:** 순수 Go SQLite(`modernc.org/sqlite`)로 로컬 DB를 구성한다. 다운로드·파싱은 기존 `internal/nec`·`internal/nesdc` 코드를 100% 재사용하고, 신규 `internal/store` 는 이미 파싱된 레코드 슬라이스(`[]nec.ResultRecord`, `[]nesdc.PollRecord`)를 받아 적재/질의만 한다. `internal/mcpserver` 는 공식 `modelcontextprotocol/go-sdk` 로 store·클라이언트를 조립해 tool 5개 + resource 1개를 노출한다.

**Tech Stack:** Go 1.26, `modernc.org/sqlite` v1.53.0 (cgo 없음), `github.com/modelcontextprotocol/go-sdk` v1.6.1, cobra.

## Global Constraints

- 모듈 경로: `github.com/JungHoonGhae/k-vote-cli`.
- **중립성(타협 불가)**: 원자료 완전 보존. 파생값은 *정의가 SQL 뷰/Go 코드에 명시된 재현 가능한 표준 변환*만. 플래그·점수·순위·"이상치"·해석 금지.
- **키리스**: API 키 발급 불필요. ingest 는 기존 `nec`/`nesdc` 클라이언트의 키리스 경로만 사용.
- **cgo 금지**: SQLite 는 `modernc.org/sqlite` (순수 Go). `import "C"` 를 새로 도입하지 않는다.
- **테스트는 네트워크 없음**: store 는 `:memory:` DB + 인라인 픽스처, mcpserver 는 `httptest.Server` + `WithBaseURL` + `NewInMemoryTransports`.
- **파생값 정의 (뷰·집계 공통, `internal/nec/aggregate.go` 와 동일)**:
  - 유효투표수 = `votes − invalid`
  - 투표율(turnout) = `votes / electorate` (electorate 0 이면 NULL/0, NaN 방지)
  - 후보 득표율(share) = `candidate votes / 유효투표수` (유효투표수 0 이면 0)
  - 집계 시 후보 합산은 town/sgg 에서만. sido/national 은 지표만, 후보 목록 비움.
- 기존 타입 시그니처 (변경 없이 소비만):
  - `nec.ResultRecord{ Sido, District, Town, Booth, VoteType string; Electorate, Votes, Invalid, Abstention int; Candidates []CandidateVote }`
  - `nec.CandidateVote{ Party, Name string; Votes int }`
  - `nec.AggLevel` (`AggNone/AggTown/AggSgg/AggSido/AggNational`), `nec.Aggregate(recs []ResultRecord, level AggLevel, byVoteType bool) []AggregatedRecord`
  - `nec.AggregatedRecord{ Level, Sido, District, Town, VoteType string; Electorate, Votes, Invalid, ValidVotes, Abstention int; Turnout float64; Candidates []CandidateShare }`
  - `nec.CandidateShare{ Party, Name string; Votes int; Share float64 }`
  - `nesdc.PollRecord{ Period, RegNo, Agency, Client, SurveyDate, Method, Frame, SampleSize, ContactRate, ResponseRate, MarginError string; PartySupport map[string]string }`
  - `nec.ParseResults(raw []byte) ([]ResultRecord, error)`, `nec.ParseResultsXLSX(raw []byte) ([]ElectionResult, error)`
  - `(*nec.Client).Download(ctx, pk, destDir) (string, error)`, `(*nec.Client).Datasets(ctx, SearchOptions) ([]Dataset, error)`
  - `(*nesdc.Client).LatestBulkXlsx(ctx, Board) (Attachment, error)`, `(*nesdc.Client).Download(ctx, Attachment, dir) (string, error)`, `nesdc.ParseBulkXlsx(path string) ([]PollRecord, error)`, `nesdc.BoardByName(name string) (Board, error)`

---

## File Structure

- `internal/store/store.go` — DB open/close, 경로 결정, `PRAGMA user_version` 마이그레이션.
- `internal/store/schema.go` — DDL 상수(테이블 + 뷰) + `SchemaDoc` (resource 노출용 스키마 설명 텍스트).
- `internal/store/ingest.go` — `IngestResults`, `IngestPolls` (dataset 단위 멱등 교체, 트랜잭션).
- `internal/store/query.go` — read-only 질의 (`QueryResult{Columns, Rows, Truncated}`).
- `internal/store/store_test.go` — 적재 멱등성·롤백·뷰↔aggregate 동치·read-only·limit 검증.
- `internal/mcpserver/server.go` — MCP 서버 조립 (tool 5 + resource 1).
- `internal/mcpserver/server_test.go` — in-memory transport 왕복 검증.
- `cmd/kvote/mcp.go` — `kvote mcp` (stdio serve).
- `cmd/kvote/db.go` — `kvote db ingest results|polls`, `kvote db query` (store 얇은 래퍼).
- `cmd/kvote/root.go` — 전역 `--db` 플래그 추가, 새 명령 등록 (Modify).
- `go.mod` / `go.sum` — 의존성 추가.

---

## Task 1: 의존성 추가 + store 스켈레톤(open/close/경로)

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`

**Interfaces:**
- Produces:
  - `store.DefaultPath() (string, error)` — OS별 기본 DB 경로.
  - `store.Open(path string) (*store.DB, error)` — 파일 열기(없으면 생성) + 스키마 보장(Task 2에서 채움; 지금은 빈 마이그레이션).
  - `store.OpenReadOnly(path string) (*store.DB, error)` — `mode=ro` DSN.
  - `(*store.DB).Close() error`, `(*store.DB).SQL() *sql.DB` (테스트/질의용 내부 접근).

- [ ] **Step 1: 의존성 추가**

Run:
```bash
go get modernc.org/sqlite@v1.53.0
go get github.com/modelcontextprotocol/go-sdk@v1.6.1
```
Expected: `go.mod` 에 두 require 추가.

- [ ] **Step 2: 실패하는 테스트 작성**

Create `internal/store/store_test.go`:
```go
package store

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesFileAndCloses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if db.SQL() == nil {
		t.Fatal("SQL() nil")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestOpenReadOnlyRejectsWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.Close()

	ro, err := OpenReadOnly(p)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer ro.Close()
	if _, err := ro.SQL().Exec("CREATE TABLE x(a)"); err == nil {
		t.Fatal("expected write to be rejected on read-only DB")
	}
}
```

- [ ] **Step 3: 테스트 실패 확인**

Run: `go test ./internal/store/ -run TestOpen -v`
Expected: FAIL — `undefined: Open` / `undefined: OpenReadOnly`.

- [ ] **Step 4: 최소 구현 작성**

Create `internal/store/store.go`:
```go
// Package store persists normalized 선거 데이터 (개표결과·여론조사) into a local
// SQLite database and exposes read-only SQL query access. It performs no network
// I/O and no interpretation — it stores 원자료 verbatim and derives only the
// standard, defined parameters (뷰 정의 참조). 판단은 소비자의 몫.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a *sql.DB opened against a kvote SQLite file.
type DB struct{ db *sql.DB }

// SQL exposes the underlying handle for queries and tests.
func (d *DB) SQL() *sql.DB { return d.db }

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

// DefaultPath returns the OS-conventional kvote DB location.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kvote", "kvote.db"), nil
}

// Open opens (creating if absent) a writable kvote DB and ensures the schema.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	sdb, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(true)")
	if err != nil {
		return nil, err
	}
	d := &DB{db: sdb}
	if err := d.migrate(); err != nil {
		sdb.Close()
		return nil, err
	}
	return d, nil
}

// OpenReadOnly opens an existing kvote DB in read-only mode; writes are rejected
// by the engine itself, so no SQL filtering is needed.
func OpenReadOnly(path string) (*DB, error) {
	sdb, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=query_only(true)")
	if err != nil {
		return nil, err
	}
	return &DB{db: sdb}, nil
}

// migrate is filled in Task 2. For now it is a no-op so Open works.
func (d *DB) migrate() error { return nil }
```

- [ ] **Step 5: 테스트 통과 확인**

Run: `go test ./internal/store/ -run TestOpen -v`
Expected: PASS (2 tests).

- [ ] **Step 6: 커밋**

```bash
go mod tidy
git add go.mod go.sum internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): SQLite open/close + read-only 모드 스켈레톤"
```

---

## Task 2: 스키마 DDL + 뷰 + 마이그레이션

**Files:**
- Create: `internal/store/schema.go`
- Modify: `internal/store/store.go` (`migrate` 채우기)
- Modify: `internal/store/store_test.go` (스키마 존재 검증 추가)

**Interfaces:**
- Consumes: `store.Open` (Task 1).
- Produces:
  - `store.SchemaSQL` (string) — 전체 DDL.
  - `store.SchemaDoc` (string) — resource 로 노출할 사람이 읽는 스키마 + 파생값 설명.
  - `store.schemaVersion` (int, unexported) — `PRAGMA user_version`.

- [ ] **Step 1: 실패하는 테스트 작성**

Add to `internal/store/store_test.go`:
```go
func TestMigrateCreatesTablesAndViews(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	want := []string{"datasets", "results", "candidates", "polls", "party_support",
		"v_results_derived", "v_agg_sgg", "v_agg_sido"}
	for _, name := range want {
		var n int
		err := db.SQL().QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE name = ?`, name).Scan(&n)
		if err != nil || n != 1 {
			t.Errorf("object %q: count=%d err=%v", name, n, err)
		}
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/store/ -run TestMigrate -v`
Expected: FAIL — 뷰/테이블 없음 (count=0).

- [ ] **Step 3: 스키마 구현**

Create `internal/store/schema.go`:
```go
package store

// schemaVersion is bumped when SchemaSQL changes incompatibly. On mismatch the
// CLI advises recreating the DB (no migration chain in v1).
const schemaVersion = 1

// SchemaSQL is the full DDL: 원자료 테이블 + 표준 파생 뷰. The view definitions ARE
// the documentation of the derived parameters — 정의가 SQL 안에 명시된다.
const SchemaSQL = `
CREATE TABLE IF NOT EXISTS datasets (
  id            INTEGER PRIMARY KEY,
  source        TEXT NOT NULL,          -- 'nec' | 'nesdc'
  public_data_pk TEXT,                  -- 다운로드 키 (nec)
  name          TEXT,                   -- 파일명/데이터셋명
  election_name TEXT,                   -- 선거명 (있으면)
  ingested_at   TEXT NOT NULL,          -- RFC3339
  row_count     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS results (
  id         INTEGER PRIMARY KEY,
  dataset_id INTEGER NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
  sido       TEXT,
  sgg        TEXT,                       -- 선거구(district)
  town       TEXT,
  booth      TEXT,
  vote_type  TEXT,                       -- 본투표|관내사전|관외사전|거소 (구조 라벨)
  electorate INTEGER,
  votes      INTEGER,
  invalid    INTEGER,
  abstention INTEGER
);
CREATE INDEX IF NOT EXISTS idx_results_dataset ON results(dataset_id);

CREATE TABLE IF NOT EXISTS candidates (
  id        INTEGER PRIMARY KEY,
  result_id INTEGER NOT NULL REFERENCES results(id) ON DELETE CASCADE,
  party     TEXT,
  name      TEXT,
  votes     INTEGER
);
CREATE INDEX IF NOT EXISTS idx_candidates_result ON candidates(result_id);

CREATE TABLE IF NOT EXISTS polls (
  id            INTEGER PRIMARY KEY,
  dataset_id    INTEGER NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
  period        TEXT,
  reg_no        TEXT,
  agency        TEXT,
  client        TEXT,
  survey_date   TEXT,
  method        TEXT,
  frame         TEXT,
  sample_size   TEXT,
  contact_rate  TEXT,
  response_rate TEXT,
  margin_error  TEXT
);
CREATE INDEX IF NOT EXISTS idx_polls_dataset ON polls(dataset_id);

CREATE TABLE IF NOT EXISTS party_support (
  id      INTEGER PRIMARY KEY,
  poll_id INTEGER NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
  party   TEXT,
  pct     TEXT
);
CREATE INDEX IF NOT EXISTS idx_party_support_poll ON party_support(poll_id);

-- 표준 파생 뷰. 정의: 유효투표수 = votes-invalid, 투표율 = votes/electorate
-- (electorate 0 → NULL), 후보 득표율 = 후보득표 / 유효투표수 (유효 0 → NULL).
CREATE VIEW IF NOT EXISTS v_results_derived AS
SELECT r.id, r.dataset_id, r.sido, r.sgg, r.town, r.booth, r.vote_type,
       r.electorate, r.votes, r.invalid, r.abstention,
       (r.votes - r.invalid) AS valid_votes,
       CASE WHEN r.electorate > 0
            THEN CAST(r.votes AS REAL) / r.electorate END AS turnout
FROM results r;

-- 선거구(sgg) 집계: 후보 합산이 의미 있는 범위. 지표 + 후보 득표율.
CREATE VIEW IF NOT EXISTS v_agg_sgg AS
WITH unit AS (
  SELECT dataset_id, sido, sgg, vote_type,
         SUM(electorate) AS electorate, SUM(votes) AS votes,
         SUM(invalid) AS invalid, SUM(abstention) AS abstention
  FROM results GROUP BY dataset_id, sido, sgg, vote_type
)
SELECT dataset_id, sido, sgg, vote_type, electorate, votes, invalid, abstention,
       (votes - invalid) AS valid_votes,
       CASE WHEN electorate > 0 THEN CAST(votes AS REAL)/electorate END AS turnout
FROM unit;

-- 시도 집계: 지역구 후보가 서로 달라 후보 합산 안 함 — 지표만.
CREATE VIEW IF NOT EXISTS v_agg_sido AS
SELECT dataset_id, sido, vote_type,
       SUM(electorate) AS electorate, SUM(votes) AS votes,
       SUM(invalid) AS invalid, SUM(abstention) AS abstention,
       (SUM(votes) - SUM(invalid)) AS valid_votes,
       CASE WHEN SUM(electorate) > 0
            THEN CAST(SUM(votes) AS REAL)/SUM(electorate) END AS turnout
FROM results GROUP BY dataset_id, sido, vote_type;

-- 전국 집계: 지표만.
CREATE VIEW IF NOT EXISTS v_agg_national AS
SELECT dataset_id, vote_type,
       SUM(electorate) AS electorate, SUM(votes) AS votes,
       SUM(invalid) AS invalid, SUM(abstention) AS abstention,
       (SUM(votes) - SUM(invalid)) AS valid_votes,
       CASE WHEN SUM(electorate) > 0
            THEN CAST(SUM(votes) AS REAL)/SUM(electorate) END AS turnout
FROM results GROUP BY dataset_id, vote_type;
`

// SchemaDoc is the human-readable schema surfaced via the kvote://schema MCP
// resource. Agents read this before writing SQL.
const SchemaDoc = `# kvote 로컬 DB 스키마

중립 원칙: 원자료(선거인수·투표수·무효·기권·후보 득표, 여론조사 원값)는 그대로 저장.
파생값은 아래 뷰의 정의(재현 가능한 산술)뿐. 판단(정상/이상)은 하지 않는다 — 소비자 몫.

## 테이블 (원자료)
- datasets(id, source['nec'|'nesdc'], public_data_pk, name, election_name, ingested_at, row_count)
- results(id, dataset_id, sido, sgg=선거구, town=읍면동, booth=투표구,
    vote_type[본투표|관내사전|관외사전|거소], electorate, votes, invalid, abstention)
- candidates(id, result_id, party, name, votes)  -- long format
- polls(id, dataset_id, period, reg_no, agency, client, survey_date, method,
    frame, sample_size, contact_rate, response_rate, margin_error)
- party_support(id, poll_id, party, pct)  -- 동적 정당 열의 long format

## 뷰 (표준 파생 — 정의는 뷰 SQL 그 자체)
- v_results_derived: 투표구 원단위 + valid_votes=votes-invalid, turnout=votes/electorate
- v_agg_sgg: 선거구 집계(지표 + 후보 합산 가능 범위)
- v_agg_sido / v_agg_national: 상위 집계. 지역구 후보가 달라 후보 합산 안 함 — 지표만.

## 후보 득표율
candidates 를 v_results_derived 와 조인해 후보득표 / valid_votes 로 계산(정의).
`
```

- [ ] **Step 4: `migrate` 채우기**

In `internal/store/store.go`, replace the `migrate` stub:
```go
// migrate applies SchemaSQL when the DB is new or matches the current version.
// On an incompatible version it errors advising recreation (no migration chain in v1).
func (d *DB) migrate() error {
	var v int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v != 0 && v != schemaVersion {
		return fmt.Errorf("db schema version %d != %d (지원 안 함) — DB 파일을 삭제하고 재생성하세요", v, schemaVersion)
	}
	if _, err := d.db.Exec(SchemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if _, err := d.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return err
	}
	return nil
}
```

- [ ] **Step 5: 테스트 통과 확인**

Run: `go test ./internal/store/ -run 'TestMigrate|TestOpen' -v`
Expected: PASS (전체).

- [ ] **Step 6: 커밋**

```bash
git add internal/store/schema.go internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): 스키마 DDL(테이블+파생 뷰) + user_version 마이그레이션"
```

---

## Task 3: 개표결과 적재 (`IngestResults`, 멱등·트랜잭션)

**Files:**
- Create: `internal/store/ingest.go`
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `store.Open`, `nec.ResultRecord`, `nec.CandidateVote`.
- Produces:
  - `type DatasetMeta struct { Source, PublicDataPk, Name, ElectionName string }`
  - `(*store.DB).IngestResults(meta DatasetMeta, recs []nec.ResultRecord) (datasetID int64, err error)` — 같은 `(source, public_data_pk)` 데이터셋이 있으면 삭제 후 재삽입(멱등). 전체 트랜잭션.

- [ ] **Step 1: 실패하는 테스트 작성**

Add to `internal/store/store_test.go`:
```go
import "github.com/JungHoonGhae/k-vote-cli/internal/nec"  // add to existing import block

func sampleResults() []nec.ResultRecord {
	return []nec.ResultRecord{
		{Sido: "서울", District: "종로구", Town: "청운효자동", Booth: "제1투", VoteType: "본투표",
			Electorate: 100, Votes: 80, Invalid: 5, Abstention: 20,
			Candidates: []nec.CandidateVote{{Party: "A당", Name: "김", Votes: 40}, {Party: "B당", Name: "이", Votes: 35}}},
		{Sido: "서울", District: "종로구", Town: "관내사전투표", Booth: "관내사전투표", VoteType: "관내사전",
			Electorate: 0, Votes: 20, Invalid: 1, Abstention: 0,
			Candidates: []nec.CandidateVote{{Party: "A당", Name: "김", Votes: 12}, {Party: "B당", Name: "이", Votes: 7}}},
	}
}

func TestIngestResultsIdempotent(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "k.db"))
	defer db.Close()
	meta := DatasetMeta{Source: "nec", PublicDataPk: "123", Name: "총선개표.csv"}

	if _, err := db.IngestResults(meta, sampleResults()); err != nil {
		t.Fatalf("ingest 1: %v", err)
	}
	if _, err := db.IngestResults(meta, sampleResults()); err != nil {
		t.Fatalf("ingest 2: %v", err)
	}
	var ds, rs, cs int
	db.SQL().QueryRow("SELECT count(*) FROM datasets").Scan(&ds)
	db.SQL().QueryRow("SELECT count(*) FROM results").Scan(&rs)
	db.SQL().QueryRow("SELECT count(*) FROM candidates").Scan(&cs)
	if ds != 1 || rs != 2 || cs != 4 {
		t.Errorf("재적재 후 중복: datasets=%d results=%d candidates=%d (want 1/2/4)", ds, rs, cs)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/store/ -run TestIngestResults -v`
Expected: FAIL — `undefined: DatasetMeta` / `IngestResults`.

- [ ] **Step 3: 구현 작성**

Create `internal/store/ingest.go`:
```go
package store

import (
	"fmt"
	"time"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
)

// DatasetMeta identifies a dataset for provenance and idempotent replacement.
type DatasetMeta struct {
	Source        string // "nec" | "nesdc"
	PublicDataPk  string
	Name          string
	ElectionName  string
}

// IngestResults replaces any existing dataset with the same (source, public_data_pk)
// and inserts the polling-unit records verbatim. All-or-nothing (transaction).
func (d *DB) IngestResults(meta DatasetMeta, recs []nec.ResultRecord) (int64, error) {
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
		rr, err := tx.Exec(
			`INSERT INTO results(dataset_id, sido, sgg, town, booth, vote_type,
			   electorate, votes, invalid, abstention) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			dsID, r.Sido, r.District, r.Town, r.Booth, r.VoteType,
			r.Electorate, r.Votes, r.Invalid, r.Abstention)
		if err != nil {
			return 0, fmt.Errorf("insert result: %w", err)
		}
		rid, _ := rr.LastInsertId()
		for _, c := range r.Candidates {
			if _, err := tx.Exec(
				`INSERT INTO candidates(result_id, party, name, votes) VALUES(?,?,?,?)`,
				rid, c.Party, c.Name, c.Votes); err != nil {
				return 0, fmt.Errorf("insert candidate: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return dsID, nil
}

// IngestPolls is implemented in Task 4.
var _ = nesdc.PollRecord{}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/store/ -run TestIngestResults -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/store/ingest.go internal/store/store_test.go
git commit -m "feat(store): 개표결과 멱등 적재(IngestResults, 트랜잭션)"
```

---

## Task 4: 여론조사 적재 (`IngestPolls`)

**Files:**
- Modify: `internal/store/ingest.go`
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `nesdc.PollRecord`, `store.DatasetMeta` (Task 3).
- Produces: `(*store.DB).IngestPolls(meta DatasetMeta, recs []nesdc.PollRecord) (datasetID int64, err error)` — `PartySupport` 맵을 `party_support` long rows 로. 멱등·트랜잭션.

- [ ] **Step 1: 실패하는 테스트 작성**

Add to `internal/store/store_test.go`:
```go
import "github.com/JungHoonGhae/k-vote-cli/internal/nesdc"  // add to import block

func TestIngestPollsIdempotent(t *testing.T) {
	db, _ := Open(filepath.Join(t.TempDir(), "k.db"))
	defer db.Close()
	meta := DatasetMeta{Source: "nesdc", PublicDataPk: "bulk", Name: "누적.xlsx"}
	recs := []nesdc.PollRecord{
		{Period: "2024", Agency: "갤럽", SampleSize: "1000",
			PartySupport: map[string]string{"A당": "40", "B당": "35"}},
	}
	if _, err := db.IngestPolls(meta, recs); err != nil {
		t.Fatalf("ingest 1: %v", err)
	}
	if _, err := db.IngestPolls(meta, recs); err != nil {
		t.Fatalf("ingest 2: %v", err)
	}
	var ps, pp int
	db.SQL().QueryRow("SELECT count(*) FROM polls").Scan(&ps)
	db.SQL().QueryRow("SELECT count(*) FROM party_support").Scan(&pp)
	if ps != 1 || pp != 2 {
		t.Errorf("재적재 후 중복: polls=%d party_support=%d (want 1/2)", ps, pp)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/store/ -run TestIngestPolls -v`
Expected: FAIL — `undefined: IngestPolls`.

- [ ] **Step 3: 구현 작성**

In `internal/store/ingest.go`, remove the `var _ = nesdc.PollRecord{}` placeholder line and append:
```go
// IngestPolls replaces any existing dataset with the same (source, public_data_pk)
// and inserts poll records plus their dynamic party-support columns (long format).
func (d *DB) IngestPolls(meta DatasetMeta, recs []nesdc.PollRecord) (int64, error) {
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
		pr, err := tx.Exec(
			`INSERT INTO polls(dataset_id, period, reg_no, agency, client, survey_date,
			   method, frame, sample_size, contact_rate, response_rate, margin_error)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			dsID, r.Period, r.RegNo, r.Agency, r.Client, r.SurveyDate,
			r.Method, r.Frame, r.SampleSize, r.ContactRate, r.ResponseRate, r.MarginError)
		if err != nil {
			return 0, fmt.Errorf("insert poll: %w", err)
		}
		pid, _ := pr.LastInsertId()
		for party, pct := range r.PartySupport {
			if _, err := tx.Exec(
				`INSERT INTO party_support(poll_id, party, pct) VALUES(?,?,?)`,
				pid, party, pct); err != nil {
				return 0, fmt.Errorf("insert party_support: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return dsID, nil
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/store/ -run TestIngest -v`
Expected: PASS (results + polls).

- [ ] **Step 5: 커밋**

```bash
git add internal/store/ingest.go internal/store/store_test.go
git commit -m "feat(store): 여론조사 멱등 적재(IngestPolls, party_support long)"
```

---

## Task 5: read-only 질의 (`Query`) + limit/truncated

**Files:**
- Create: `internal/store/query.go`
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `store.OpenReadOnly`, `store.IngestResults`.
- Produces:
  - `type QueryResult struct { Columns []string; Rows [][]any; Truncated bool }`
  - `(*store.DB).Query(sql string, limit int) (*QueryResult, error)` — limit 초과분은 자르고 `Truncated=true`. limit<=0 이면 기본 1000.

- [ ] **Step 1: 실패하는 테스트 작성**

Add to `internal/store/store_test.go`:
```go
func TestQueryReadsAndTruncates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := Open(p)
	db.IngestResults(DatasetMeta{Source: "nec", PublicDataPk: "1"}, sampleResults())
	db.Close()

	ro, err := OpenReadOnly(p)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer ro.Close()

	qr, err := ro.Query("SELECT sido, votes FROM results ORDER BY id", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(qr.Columns) != 2 || qr.Columns[0] != "sido" {
		t.Errorf("columns = %v", qr.Columns)
	}
	if len(qr.Rows) != 1 || !qr.Truncated {
		t.Errorf("rows=%d truncated=%v (want 1 / true)", len(qr.Rows), qr.Truncated)
	}
}

func TestQueryRejectsWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := Open(p)
	db.Close()
	ro, _ := OpenReadOnly(p)
	defer ro.Close()
	if _, err := ro.Query("INSERT INTO datasets(source, ingested_at) VALUES('x','y')", 10); err == nil {
		t.Fatal("expected write SQL to be rejected on read-only DB")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/store/ -run TestQuery -v`
Expected: FAIL — `undefined: QueryResult` / `Query`.

- [ ] **Step 3: 구현 작성**

Create `internal/store/query.go`:
```go
package store

// QueryResult is a generic tabular result: column names + rows of scalar values,
// with Truncated set when more rows existed than the limit allowed.
type QueryResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated"`
}

const defaultQueryLimit = 1000

// Query runs a read-only SQL statement and returns up to limit rows. Writes are
// rejected by the engine (open the DB via OpenReadOnly). limit<=0 uses the default.
func (d *DB) Query(sql string, limit int) (*QueryResult, error) {
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	rows, err := d.db.Query(sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := &QueryResult{Columns: cols, Rows: [][]any{}}
	for rows.Next() {
		if len(out.Rows) >= limit {
			out.Truncated = true
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		// []byte → string 로 정규화 (JSON 직렬화·가독성).
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		out.Rows = append(out.Rows, vals)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/store/ -run TestQuery -v`
Expected: PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/store/query.go internal/store/store_test.go
git commit -m "feat(store): read-only Query + limit/truncated"
```

---

## Task 6: 뷰 ↔ aggregate.go 동치성 테스트 (정의 드리프트 방지)

**Files:**
- Modify: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `store.IngestResults`, `store.Query`, `nec.Aggregate`, `nec.AggSgg`.
- Produces: (테스트만)

- [ ] **Step 1: 동치성 테스트 작성**

Add to `internal/store/store_test.go`:
```go
// v_agg_sgg 뷰와 nec.Aggregate(AggSgg) 는 같은 정의의 두 구현이다. 같은 픽스처에서
// 지표·투표율이 일치해야 정의 드리프트가 없다.
func TestViewMatchesAggregate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := Open(p)
	recs := sampleResults()
	db.IngestResults(DatasetMeta{Source: "nec", PublicDataPk: "1"}, recs)
	db.Close()

	// Go 경로: 선거구 집계 (by-votetype=false → vote_type 합쳐짐).
	aggs := nec.Aggregate(recs, nec.AggSgg, false)
	want := map[string][3]int{} // key = sido|sgg → {electorate, votes, invalid}
	for _, a := range aggs {
		want[a.Sido+"|"+a.District] = [3]int{a.Electorate, a.Votes, a.Invalid}
	}

	// SQL 경로: v_agg_sgg 는 vote_type 별이므로 sido,sgg 로 다시 합산해 비교.
	ro, _ := OpenReadOnly(p)
	defer ro.Close()
	qr, err := ro.Query(
		`SELECT sido, sgg, SUM(electorate), SUM(votes), SUM(invalid)
		 FROM v_agg_sgg GROUP BY sido, sgg ORDER BY sido, sgg`, 100)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(qr.Rows) != len(want) {
		t.Fatalf("행수 불일치: sql=%d go=%d", len(qr.Rows), len(want))
	}
	for _, row := range qr.Rows {
		key := toStr(row[0]) + "|" + toStr(row[1])
		got := [3]int{toInt(row[2]), toInt(row[3]), toInt(row[4])}
		if got != want[key] {
			t.Errorf("%s: sql=%v go=%v", key, got, want[key])
		}
	}
}

func toStr(v any) string { s, _ := v.(string); return s }
func toInt(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	}
	return 0
}
```

- [ ] **Step 2: 테스트 통과 확인**

Run: `go test ./internal/store/ -run TestViewMatchesAggregate -v`
Expected: PASS. (실패 시 뷰 정의와 aggregate.go 정의 중 하나가 어긋난 것 — 뷰 SQL 을 aggregate.go 정의에 맞춘다.)

- [ ] **Step 3: 전체 store 테스트 확인**

Run: `go test ./internal/store/ -v`
Expected: PASS (전체).

- [ ] **Step 4: 커밋**

```bash
git add internal/store/store_test.go
git commit -m "test(store): v_agg_sgg 뷰 ↔ nec.Aggregate 동치성 (정의 드리프트 방지)"
```

---

## Task 7: MCP 서버 — query tool + schema resource

**Files:**
- Create: `internal/mcpserver/server.go`
- Create: `internal/mcpserver/server_test.go`

**Interfaces:**
- Consumes: `store.OpenReadOnly`, `store.Open`, `store.SchemaDoc`, `store.QueryResult`, go-sdk `mcp`.
- Produces:
  - `type Deps struct { DBPath string; NEC *nec.Client; NESDC *nesdc.Client }`
  - `mcpserver.New(deps Deps) *mcp.Server` — tool·resource 등록된 서버.
  - `mcpserver.Serve(ctx context.Context, deps Deps) error` — stdio transport 로 Run.
- 주의: go-sdk v1.6.1 API — `mcp.NewServer(&mcp.Implementation{...}, nil)`, `mcp.AddTool[In,Out](s, &mcp.Tool{...}, handler)`, `s.AddResource(&mcp.Resource{...}, handler)`, `s.Run(ctx, &mcp.StdioTransport{})`. ToolHandlerFor 시그니처: `func(ctx, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)`.

- [ ] **Step 1: 실패하는 테스트 작성**

Create `internal/mcpserver/server_test.go`:
```go
package mcpserver

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestQueryToolRoundTrip(t *testing.T) {
	// seed a DB
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := store.Open(p)
	db.SQL().Exec(`INSERT INTO datasets(source, ingested_at, row_count) VALUES('nec','now',0)`)
	db.Close()

	srv := New(Deps{DBPath: p})
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "query",
		Arguments: map[string]any{"sql": "SELECT count(*) AS n FROM datasets"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res.Content)
	}
}

func TestSchemaResource(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.db")
	db, _ := store.Open(p)
	db.Close()
	srv := New(Deps{DBPath: p})
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	srv.Connect(ctx, st, nil)
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, _ := client.Connect(ctx, ct, nil)
	defer cs.Close()

	rr, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "kvote://schema"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(rr.Contents) == 0 || rr.Contents[0].Text == "" {
		t.Fatal("schema resource empty")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/mcpserver/ -run 'TestQueryTool|TestSchema' -v`
Expected: FAIL — `undefined: New` / `Deps`.

- [ ] **Step 3: 서버 구현 (query tool + schema resource)**

Create `internal/mcpserver/server.go`:
```go
// Package mcpserver exposes kvote's data over the Model Context Protocol: 탐색·
// 키리스 수집·read-only SQL 질의를 tool 로 제공한다. 데이터·파생값은 store 계층이
// 정의하고, 이 패키지는 조립만 한다 (판단 없음 — 중립).
package mcpserver

import (
	"context"
	"fmt"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Deps carries the collaborators the server needs.
type Deps struct {
	DBPath string
	NEC    *nec.Client
	NESDC  *nesdc.Client
}

// --- tool I/O types (struct → JSON schema 자동 추론) ---

type queryIn struct {
	SQL   string `json:"sql" jsonschema:"read-only SQL statement to execute against the kvote DB (see kvote://schema)"`
	Limit int    `json:"limit,omitempty" jsonschema:"max rows to return (default 1000)"`
}

// New builds the MCP server with all tools and the schema resource registered.
func New(deps Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "kvote",
		Title:   "kvote — 한국 선거 공개 데이터",
		Version: "0.1.0",
	}, nil)

	// query: read-only SQL passthrough.
	mcp.AddTool(s, &mcp.Tool{
		Name:        "query",
		Description: "kvote 로컬 DB에 read-only SQL을 실행한다. 스키마·파생값 정의는 먼저 kvote://schema 리소스를 읽을 것. 쓰기 SQL은 엔진이 거부한다.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in queryIn) (*mcp.CallToolResult, *store.QueryResult, error) {
		db, err := store.OpenReadOnly(deps.DBPath)
		if err != nil {
			return errResult(fmt.Sprintf("DB 열기 실패: %v — 먼저 ingest_results/ingest_polls 로 적재하세요", err)), nil, nil
		}
		defer db.Close()
		qr, err := db.Query(in.SQL, in.Limit)
		if err != nil {
			return errResult(fmt.Sprintf("질의 오류: %v", err)), nil, nil
		}
		return nil, qr, nil
	})

	registerIngestTools(s, deps) // Task 8
	registerSearchTools(s, deps) // Task 9

	// schema resource.
	s.AddResource(&mcp.Resource{
		Name:        "schema",
		URI:         "kvote://schema",
		MIMEType:    "text/markdown",
		Description: "kvote DB 테이블·뷰 스키마와 표준 파생값 정의. query 전에 읽으세요.",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      "kvote://schema",
			MIMEType: "text/markdown",
			Text:     store.SchemaDoc,
		}}}, nil
	})

	return s
}

// errResult wraps a user-facing error message as a non-fatal tool error result.
func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}

// Serve runs the server over stdio until the client disconnects or ctx is cancelled.
func Serve(ctx context.Context, deps Deps) error {
	return New(deps).Run(ctx, &mcp.StdioTransport{})
}
```

**Note (구현자):** `registerIngestTools`·`registerSearchTools` 는 Task 8·9 에서 같은 패키지에 추가된다. **이 태스크만 단독 컴파일하려면** 두 호출을 잠시 주석 처리하고 테스트한 뒤, Task 8·9 에서 주석을 해제하라. (권장: Task 7→8→9 를 연속 실행.) 임시로 아래 빈 스텁을 `server.go` 하단에 두면 컴파일된다:
```go
func registerIngestTools(s *mcp.Server, deps Deps) {}
func registerSearchTools(s *mcp.Server, deps Deps) {}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/mcpserver/ -run 'TestQueryTool|TestSchema' -v`
Expected: PASS. (컴파일 에러 시 위 빈 스텁이 있는지 확인.)

- [ ] **Step 5: 커밋**

```bash
git add internal/mcpserver/server.go internal/mcpserver/server_test.go
git commit -m "feat(mcpserver): MCP 서버 조립 — query tool + kvote://schema 리소스"
```

---

## Task 8: MCP ingest tools (ingest_results, ingest_polls)

**Files:**
- Create: `internal/mcpserver/ingest.go`
- Modify: `internal/mcpserver/server.go` (빈 스텁 `registerIngestTools` 제거)
- Modify: `internal/mcpserver/server_test.go`

**Interfaces:**
- Consumes: `deps.NEC.Download`, `nec.ParseResults`, `deps.NESDC.LatestBulkXlsx`/`Download`/`nesdc.ParseBulkXlsx`, `store.Open`/`IngestResults`/`IngestPolls`.
- Produces: `registerIngestTools(s *mcp.Server, deps Deps)` (실 구현).

- [ ] **Step 1: 실패하는 테스트 작성 (httptest 픽스처로 ingest_results 왕복)**

Add to `internal/mcpserver/server_test.go`. 기존 nec 테스트가 쓰는 픽스처 서버 패턴을 재사용한다. `internal/nec/nec_test.go` 의 다운로드 3단계 핸들러를 참고해 최소 CSV 를 서빙하는 `httptest.Server` 를 세운 뒤 `nec.New(nec.WithBaseURL(ts.URL), ...)` 로 `deps.NEC` 를 구성하고 `ingest_results` tool 을 호출, 이후 `query` 로 결과 행 존재를 확인한다.

```go
// TestIngestResultsTool 는 httptest 픽스처에서 다운로드→파싱→적재→질의를 왕복 검증한다.
// 픽스처 핸들러는 internal/nec/nec_test.go 의 TestDownload 계열과 동일한 3단계
// (fileData.do → selectFileDataDownload.do → fileDownload.do) 를 최소 구현한다.
func TestIngestResultsTool(t *testing.T) {
	t.Skip("픽스처 핸들러는 구현 시 internal/nec/nec_test.go 패턴을 복제해 채운다")
}
```

**구현자 지침:** 위 `t.Skip` 을 제거하고, `internal/nec/nec_test.go` 에서 `selectFileDataDownload.do`·`fileDownload.do`·`fileData.do` 를 서빙하는 핸들러를 그대로 가져와 CSV 본문만 아래 최소 long-format 으로 바꾼다:
```
시도명,선거구명,법정읍면동명,투표구명,후보자,득표수
서울,종로구,청운효자동,제1투,선거인수,100
서울,종로구,청운효자동,제1투,투표수,80
서울,종로구,청운효자동,제1투,무효투표수,5
서울,종로구,청운효자동,제1투,기권자수,20
서울,종로구,청운효자동,제1투,A당 김,40
서울,종로구,청운효자동,제1투,B당 이,35
```
그런 다음 tool 호출 → `query "SELECT count(*) FROM results"` 로 1행 이상 확인.

- [ ] **Step 2: 테스트 실패(또는 skip) 확인**

Run: `go test ./internal/mcpserver/ -run TestIngestResultsTool -v`
Expected: SKIP (스텁) 또는 구현 후 FAIL — `registerIngestTools` 미구현.

- [ ] **Step 3: ingest 구현**

Create `internal/mcpserver/ingest.go`:
```go
package mcpserver

import (
	"context"
	"fmt"
	"os"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ingestResultsIn struct {
	PK string `json:"pk" jsonschema:"data.go.kr publicDataPk of the 개표결과 file dataset to download and ingest"`
}
type ingestSummary struct {
	DatasetID int64  `json:"datasetId"`
	Rows      int    `json:"rows"`
	Message   string `json:"message"`
}
type ingestPollsIn struct {
	Board string `json:"board,omitempty" jsonschema:"NESDC board name carrying the cumulative master xlsx (default 'data')"`
}

func registerIngestTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "ingest_results",
		Description: "data.go.kr publicDataPk 의 개표결과(CSV)를 키 없이 내려받아 정규화 후 로컬 DB에 적재한다(멱등). XLSX 전용 데이터셋은 미지원.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ingestResultsIn) (*mcp.CallToolResult, *ingestSummary, error) {
		dir, err := os.MkdirTemp("", "kvote-mcp-")
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
		recs, err := nec.ParseResults(raw)
		if err != nil {
			return errResult(fmt.Sprintf("정규화 실패(XLSX 전용일 수 있음 — nec pull 로 원본 확인): %v", err)), nil, nil
		}
		db, err := store.Open(deps.DBPath)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		defer db.Close()
		dsID, err := db.IngestResults(store.DatasetMeta{Source: "nec", PublicDataPk: in.PK, Name: path}, recs)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &ingestSummary{DatasetID: dsID, Rows: len(recs),
			Message: fmt.Sprintf("%d개 투표구 적재", len(recs))}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ingest_polls",
		Description: "NESDC 누적 마스터 엑셀(전국 여론조사)을 내려받아 정규화 후 로컬 DB에 적재한다(멱등).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ingestPollsIn) (*mcp.CallToolResult, *ingestSummary, error) {
		boardName := in.Board
		if boardName == "" {
			boardName = "data"
		}
		board, err := nesdc.BoardByName(boardName)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		att, err := deps.NESDC.LatestBulkXlsx(ctx, board)
		if err != nil {
			return errResult(fmt.Sprintf("최신 엑셀 조회 실패: %v", err)), nil, nil
		}
		dir, err := os.MkdirTemp("", "kvote-mcp-poll-")
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		defer os.RemoveAll(dir)
		path, err := deps.NESDC.Download(ctx, att, dir)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		recs, err := nesdc.ParseBulkXlsx(path)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		db, err := store.Open(deps.DBPath)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		defer db.Close()
		dsID, err := db.IngestPolls(store.DatasetMeta{Source: "nesdc", PublicDataPk: "bulk-" + boardName, Name: path}, recs)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &ingestSummary{DatasetID: dsID, Rows: len(recs),
			Message: fmt.Sprintf("%d건 적재", len(recs))}, nil
	})
}
```

- [ ] **Step 4: server.go 의 빈 스텁 제거**

In `internal/mcpserver/server.go`, delete the placeholder line `func registerIngestTools(s *mcp.Server, deps Deps) {}` (실 구현이 ingest.go 에 생김).

- [ ] **Step 5: 테스트 통과 확인**

Run: `go test ./internal/mcpserver/ -run 'TestIngestResultsTool|TestQueryTool|TestSchema' -v`
Expected: PASS.

- [ ] **Step 6: 커밋**

```bash
git add internal/mcpserver/ingest.go internal/mcpserver/server.go internal/mcpserver/server_test.go
git commit -m "feat(mcpserver): ingest_results·ingest_polls tool (키리스 수집→적재)"
```

---

## Task 9: MCP search tools (search_datasets, list_elections)

**Files:**
- Create: `internal/mcpserver/search.go`
- Modify: `internal/mcpserver/server.go` (빈 스텁 `registerSearchTools` 제거)
- Modify: `internal/mcpserver/server_test.go`

**Interfaces:**
- Consumes: `deps.NEC.Datasets(ctx, nec.SearchOptions{Keyword})`, `deps.NEC.LatestDataset(ctx, keyword)`.
- Produces: `registerSearchTools(s *mcp.Server, deps Deps)`.
- 참고: `list_elections` 는 스펙의 "선거 목록" 을 키워드 기반 최신 데이터셋 조회(`LatestDataset`)로 구현한다 — OpenAPI 키가 필요한 `nec.Elections` 대신 키리스 경로를 쓴다.

- [ ] **Step 1: 실패하는 테스트 작성**

Add to `internal/mcpserver/server_test.go`. `internal/nec/nec_test.go` 의 `selectDataSetList.do` 픽스처를 재사용해 `search_datasets` 호출이 성공 결과를 돌려주는지 확인:
```go
func TestSearchDatasetsTool(t *testing.T) {
	t.Skip("픽스처 핸들러는 internal/nec/nec_test.go 의 Datasets 테스트 패턴을 복제해 채운다")
}
```
**구현자 지침:** `t.Skip` 제거 후, nec 테스트의 `selectDataSetList.do` HTML 픽스처를 서빙하는 `httptest.Server` 로 `deps.NEC` 를 구성하고 tool 을 호출, `res.IsError == false` 와 최소 1개 dataset 반환을 확인한다.

- [ ] **Step 2: 테스트 실패(skip) 확인**

Run: `go test ./internal/mcpserver/ -run TestSearchDatasetsTool -v`
Expected: SKIP 또는 FAIL.

- [ ] **Step 3: search 구현**

Create `internal/mcpserver/search.go`:
```go
package mcpserver

import (
	"context"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type searchIn struct {
	Keyword string `json:"keyword" jsonschema:"free-text query for NEC file datasets on data.go.kr"`
}
type searchOut struct {
	Datasets []nec.Dataset `json:"datasets"`
}
type latestIn struct {
	Keyword string `json:"keyword" jsonschema:"one election type keyword, e.g. '지방선거 개표결과'"`
}
type latestOut struct {
	Refs []nec.LatestRef `json:"refs"`
}

func registerSearchTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_datasets",
		Description: "선관위가 data.go.kr 에 공개한 파일 데이터셋을 키워드로 검색한다. 반환된 publicDataPk 를 ingest_results 에 넘긴다.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, *searchOut, error) {
		ds, err := deps.NEC.Datasets(ctx, nec.SearchOptions{Keyword: in.Keyword})
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return nil, &searchOut{Datasets: ds}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_elections",
		Description: "선거종류 키워드로 각 소스(data.go.kr·개방포털)의 최신 회차 데이터셋을 찾는다. 키리스.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in latestIn) (*mcp.CallToolResult, *latestOut, error) {
		refs := deps.NEC.LatestDataset(ctx, in.Keyword)
		return nil, &latestOut{Refs: refs}, nil
	})
}
```

- [ ] **Step 4: server.go 의 빈 스텁 제거**

In `internal/mcpserver/server.go`, delete `func registerSearchTools(s *mcp.Server, deps Deps) {}`.

- [ ] **Step 5: 테스트 통과 확인**

Run: `go test ./internal/mcpserver/ -v`
Expected: PASS (전체).

- [ ] **Step 6: 커밋**

```bash
git add internal/mcpserver/search.go internal/mcpserver/server.go internal/mcpserver/server_test.go
git commit -m "feat(mcpserver): search_datasets·list_elections tool (키리스 탐색)"
```

---

## Task 10: CLI — `kvote mcp` + `kvote db` + 전역 `--db`

**Files:**
- Create: `cmd/kvote/mcp.go`
- Create: `cmd/kvote/db.go`
- Modify: `cmd/kvote/root.go`

**Interfaces:**
- Consumes: `mcpserver.Serve`, `store.Open`/`OpenReadOnly`/`DefaultPath`, `newNECClient()`/`newNESDCClient()` (root.go 기존 헬퍼), `nec.ParseResults`, `nesdc.ParseBulkXlsx`.
- Produces:
  - `mcpCmd() *cobra.Command`, `dbCmd() *cobra.Command`, 전역 `flagDBPath` + `resolveDBPath() (string, error)`.

- [ ] **Step 1: root.go 에 --db 플래그 + 명령 등록**

In `cmd/kvote/root.go`, add to the `var (...)` block:
```go
	flagDBPath string
```
In `init()`, after the existing persistent flags:
```go
	pf.StringVar(&flagDBPath, "db", "", "로컬 DB 경로 (기본: OS 설정 디렉터리/kvote/kvote.db)")
```
and register commands (after `rootCmd.AddCommand(doctorCmd())`):
```go
	rootCmd.AddCommand(mcpCmd())
	rootCmd.AddCommand(dbCmd())
```
Add helper at the bottom of root.go:
```go
// resolveDBPath returns the --db override or the OS-default path.
func resolveDBPath() (string, error) {
	if flagDBPath != "" {
		return flagDBPath, nil
	}
	return store.DefaultPath()
}
```
Add `"github.com/JungHoonGhae/k-vote-cli/internal/store"` to root.go imports.

- [ ] **Step 2: `kvote mcp` 명령 작성**

Create `cmd/kvote/mcp.go`:
```go
package main

import (
	"context"

	"github.com/JungHoonGhae/k-vote-cli/internal/mcpserver"
	"github.com/spf13/cobra"
)

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "MCP 서버 실행 (stdio) — AI 에이전트가 탐색·수집·SQL 질의",
		Long: `kvote 를 Model Context Protocol 서버로 노출합니다(stdio).
AI 에이전트가 search_datasets / list_elections / ingest_results / ingest_polls /
query tool 과 kvote://schema 리소스로 한국 선거 공개 데이터를 다룹니다. 키리스·중립.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dbPath, err := resolveDBPath()
			if err != nil {
				return err
			}
			return mcpserver.Serve(context.Background(), mcpserver.Deps{
				DBPath: dbPath,
				NEC:    newNECClient(),
				NESDC:  newNESDCClient(),
			})
		},
	}
}
```

- [ ] **Step 3: `kvote db` 명령 작성**

Create `cmd/kvote/db.go`:
```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
	"github.com/JungHoonGhae/k-vote-cli/internal/output"
	"github.com/JungHoonGhae/k-vote-cli/internal/store"
	"github.com/spf13/cobra"
)

func dbCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "db",
		Short: "로컬 SQLite 데이터셋 적재·질의 (MCP query 와 같은 store)",
	}
	c.AddCommand(dbIngestCmd(), dbQueryCmd())
	return c
}

func dbIngestCmd() *cobra.Command {
	ingest := &cobra.Command{Use: "ingest", Short: "데이터를 로컬 DB에 적재"}

	results := &cobra.Command{
		Use:   "results <publicDataPk>",
		Short: "개표결과 CSV를 내려받아 적재 (멱등)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, err := resolveDBPath()
			if err != nil {
				return err
			}
			dir, err := os.MkdirTemp("", "kvote-db-")
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
			recs, err := nec.ParseResults(raw)
			if err != nil {
				return err
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			id, err := db.IngestResults(store.DatasetMeta{Source: "nec", PublicDataPk: args[0], Name: path}, recs)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "적재 완료: dataset=%d, %d개 투표구\n", id, len(recs))
			return nil
		},
	}

	polls := &cobra.Command{
		Use:   "polls",
		Short: "NESDC 누적 여론조사 엑셀을 내려받아 적재 (멱등)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dbPath, err := resolveDBPath()
			if err != nil {
				return err
			}
			board, err := nesdc.BoardByName("data")
			if err != nil {
				return err
			}
			client := newNESDCClient()
			ctx := context.Background()
			att, err := client.LatestBulkXlsx(ctx, board)
			if err != nil {
				return err
			}
			dir, err := os.MkdirTemp("", "kvote-db-poll-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			path, err := client.Download(ctx, att, dir)
			if err != nil {
				return err
			}
			recs, err := nesdc.ParseBulkXlsx(path)
			if err != nil {
				return err
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			id, err := db.IngestPolls(store.DatasetMeta{Source: "nesdc", PublicDataPk: "bulk-data", Name: path}, recs)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "적재 완료: dataset=%d, %d건\n", id, len(recs))
			return nil
		},
	}

	ingest.AddCommand(results, polls)
	return ingest
}

func dbQueryCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "query <sql>",
		Short: "로컬 DB에 read-only SQL 질의",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat()
			if err != nil {
				return err
			}
			dbPath, err := resolveDBPath()
			if err != nil {
				return err
			}
			db, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			qr, err := db.Query(args[0], limit)
			if err != nil {
				return err
			}
			if format == output.Table {
				rows := make([][]string, len(qr.Rows))
				for i, r := range qr.Rows {
					cells := make([]string, len(r))
					for j, v := range r {
						cells[j] = fmt.Sprintf("%v", v)
					}
					rows[i] = cells
				}
				return output.WriteTable(cmd.OutOrStdout(), qr.Columns, rows)
			}
			return output.WriteJSON(cmd.OutOrStdout(), qr)
		},
	}
	c.Flags().IntVar(&limit, "limit", 1000, "최대 반환 행수")
	return c
}
```

**구현자 주의:** `output.WriteTable` 시그니처를 확인하라 (`cmd/kvote/nesdc.go` 의 `renderBulk` 가 헤더 `[]string` + 행 `[][]string` 을 넘긴다). 시그니처가 다르면 그 호출부 형태에 맞춘다.

- [ ] **Step 4: 빌드 + 스모크**

Run:
```bash
make build
./bin/kvote mcp --help
./bin/kvote db --help
./bin/kvote db query --help
```
Expected: 각 명령 도움말 출력, 에러 없음.

- [ ] **Step 5: 전체 테스트 + vet**

Run:
```bash
go build ./... && go vet ./... && go test ./...
```
Expected: 전부 통과.

- [ ] **Step 6: 커밋**

```bash
make fmt
git add cmd/kvote/mcp.go cmd/kvote/db.go cmd/kvote/root.go
git commit -m "feat(cmd): kvote mcp (stdio) + kvote db ingest/query + 전역 --db"
```

---

## Task 11: 문서 — CLAUDE.md·README 반영

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md` (있으면 — 명령 목록·MCP 사용법 섹션)

**Interfaces:** (문서만)

- [ ] **Step 1: CLAUDE.md 아키텍처 맵 갱신**

`CLAUDE.md` 의 아키텍처 섹션에 다음을 추가:
```
internal/store/     로컬 SQLite 통합 데이터셋 (modernc, 키리스·중립)
  store.go          open/close, 경로 결정, read-only 모드, user_version 마이그레이션
  schema.go         DDL(원자료 테이블 + 표준 파생 뷰) + SchemaDoc(MCP 리소스 텍스트)
  ingest.go         []ResultRecord / []PollRecord → dataset 단위 멱등 적재(트랜잭션)
  query.go          read-only SQL 질의 → {columns, rows, truncated}
internal/mcpserver/ MCP 서버 (stdio, modelcontextprotocol/go-sdk)
  server.go         조립 + query tool + kvote://schema 리소스
  ingest.go         ingest_results / ingest_polls tool (키리스 수집→적재)
  search.go         search_datasets / list_elections tool (키리스 탐색)
```
그리고 명령어 섹션에 라이브 예시 추가:
```bash
./bin/kvote mcp                          # MCP 서버 (stdio) — AI 에이전트 연결
./bin/kvote db ingest results <pk>       # 개표결과 로컬 DB 적재
./bin/kvote db query "SELECT ..."        # read-only SQL 질의
```

- [ ] **Step 2: 핵심 설계 포인트 한 줄 추가**

`CLAUDE.md` 의 "핵심 설계 포인트" 목록에:
```
- **로컬 DB 는 중립의 연장**: `internal/store` 는 원자료를 그대로 저장하고, 파생값은 뷰 SQL
  정의(유효표·투표율·득표율)뿐. MCP `query` 는 **read-only 연결**(`mode=ro`)이라 쓰기 SQL 은
  엔진이 거부한다. 뷰 정의는 `aggregate.go` 와 동치 테스트로 드리프트를 막는다.
```

- [ ] **Step 3: README 갱신 (파일이 있으면)**

README 에 MCP 사용법 섹션을 추가 (에이전트 등록 예시). README 가 없으면 이 단계는 skip.

- [ ] **Step 4: 커밋**

```bash
git add CLAUDE.md README.md
git commit -m "docs: internal/store·mcpserver + kvote mcp/db 명령 반영"
```

---

## Self-Review (작성자 확인 결과)

**1. Spec coverage:**
- 아키텍처(store/mcpserver/mcp.go/db.go) → Task 1–10 ✅
- 스키마 테이블·뷰 → Task 2 ✅ / 파생값 정의 → Global Constraints + 뷰 ✅
- MCP tool 5개(search_datasets, list_elections, ingest_results, ingest_polls, query) → Task 7·8·9 ✅
- schema resource → Task 7 ✅
- read-only 엔진 차단 → Task 1·5 (검증), Task 7 (query tool) ✅
- 멱등·트랜잭션·롤백 → Task 3·4 ✅
- aggregate ↔ 뷰 동치 → Task 6 ✅
- CLI(`kvote mcp`, `kvote db`, `--db`) → Task 10 ✅
- 테스트 전략(:memory:, httptest, InMemoryTransports) → Task 1–9 ✅
- YAGNI(구조화 tool·SSE·P3·마이그레이션 체인 제외) → 계획에 미포함 ✅

**2. Placeholder scan:** ingest/search MCP 테스트에 `t.Skip` 스텁 + 상세 구현 지침을 둠(픽스처는 기존 nec 테스트 복제이므로 코드 전량 재기재 대신 출처 지정). 그 외 실제 코드 전량 기재.

**3. Type consistency:** `DatasetMeta`·`QueryResult`·`Deps`·`ingestSummary` 시그니처가 Task 간 일치. `nec.ResultRecord.District` → `results.sgg` 매핑 명시. `errResult` 헬퍼 Task 7 정의 후 8·9 재사용.

**주의 사항 (구현자):**
- go-sdk v1.6.1 의 `CallToolResult.IsError`·`TextContent`·`CallToolParams`·`ReadResourceParams` 등 정확한 필드/타입은 구현 착수 시 `go doc` 로 확인(버전 소폭 차이 가능). 시그니처가 다르면 컴파일 에러 메시지에 맞춰 조정.
- `output.WriteTable` 시그니처는 `renderBulk` 호출부를 기준으로 맞출 것.
