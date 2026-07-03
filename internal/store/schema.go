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

-- 선거구(sgg) 집계: 지표만(sido·sgg·vote_type 별 SUM + 파생값). 후보 열은 없음 —
-- 후보 합산이 필요하면 candidates 를 result_id 로 조인해 별도 계산(SchemaDoc 참고).
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
- v_agg_sgg: 선거구 집계(지표만). 후보 합산은 candidates 를 result_id 로 조인해 계산
- v_agg_sido / v_agg_national: 상위 집계. 지역구 후보가 달라 후보 합산 안 함 — 지표만.

## 후보 득표율
candidates 를 v_results_derived 와 조인해 후보득표 / valid_votes 로 계산(정의).

- turnout(id, dataset_id, election, category[표본/표본-일반…], region_level[구시군|선거구],
    sido, region, gender[합계|남자|여자], age_group[합계|18세|20-24세…], electorate, voters,
    rate)  -- rate 는 소스가 보고한 투표율 원자료
- v_turnout_derived: 위 + rate_computed = voters/electorate*100 (electorate 0 → NULL).
    rate_reported(소스값) 와 rate_computed(재계산) 를 나란히 — 판단은 소비자.
`
