# kvote P3 설계 — `nec turnout-analysis`: 성별·연령대별 투표율 정규화 (키리스)

작성일: 2026-07-03
상태: 승인됨 (브레인스토밍 완료, 구현 계획 대기)
선행: `2026-06-21-kvote-coverage-roadmap-design.md` 의 P3, `2026-07-02-kvote-mcp-sqlite-design.md`(store/MCP 패턴 재사용)

## 1. 목표와 중복성 검증

로드맵 P3 "투표율(turnout) 데이터셋 정규화"를 구체화한다. **핵심은 개표결과에서 파생 불가능한
독립 축을 제공하는 것.**

- **중복 우려 해소 (실데이터 확인)**: 기존 `nec turnout`(OpenAPI 래퍼, 키 기반, sgId 인자)이
  주는 최종 집계 투표율은 `nec results --aggregate`로 파생 가능해 실제로 중복이다. 그러나
  data.go.kr이 파일로 공개하는 **"투표율 분석"** 데이터셋은 전혀 다른 물건이다 — ZIP 안 ~19개
  XLSX로, 핵심은 **성별 × 연령대 × 지역별 투표율** 교차표다. 개표결과에는 "누가 투표했는가"의
  인구통계 축이 없으므로 이는 **파생 불가능한 독립 파라미터 축**이다.
- **미션 정합**: 이 인구통계 축은 P4 여론조사 교차표(성별·연령)와 같은 차원이라, 소비자가
  *여론조사 ↔ 투표율 ↔ 개표*를 나란히 비교할 수 있게 된다. 중립 원칙(차원을 넓게 제공, 판단은
  소비자) 그대로.
- **중립성(타협 불가)**: 원자료 완전 보존. 소스가 제공한 투표율(`Rate`)은 우리가 파생한 값이
  아니라 원자료로 그대로 저장. 우리가 더하는 파생값은 정의가 명시된 표준 변환(`rate_computed =
  voters/electorate`) 하나뿐이며, 소스 보고값과 별개 컬럼으로 명확히 구분한다. 플래그·점수·해석 없음.
- **키리스**: `nec pull`과 동일한 키 없는 파일 다운로드 경로만 사용.

## 2. 데이터 구조 (실데이터 기준)

`nec pull <pk>`가 반환하는 것은 **ZIP**이다 (예: 제22대 국회의원선거 투표율 분석). ZIP 내부:

```
01_전체 선거인 표본조사/  02_성별·연령대별 투표율(구시군별).xlsx, 05_...(선거구별).xlsx, ...
02_선거일 투표/           02_성별·연령대별(구시군별).xlsx, 05_...(선거구별).xlsx, ...
03_사전투표/              (레이아웃 상이 — 이번 범위 밖)
04_재외투표/              (레이아웃 상이 — 이번 범위 밖)
```

**대상 파일**(균일한 성별·연령대 교차표): `01_전체 선거인 표본조사`·`02_선거일 투표` 아래의
`성별·연령대별 투표율` 파일들(구시군별·선거구별). 이들은 동일 앵커 구조를 공유한다:

- **시트 = 시도** (17개: 서울/부산/…/세종/경기…).
- **row0 제목**: `성별·연령대별 투표율(구시군별)` 또는 `(선거구별)` → `RegionLevel` 도출.
- **대괄호 마커** (예: `[표본-일반][서울특별시]`, `[표본][서울특별시]`): 대괄호 태그 =
  `Category`(verbatim), 뒤 = 시도(시트명과 중복 — 시트명 사용).
- **`구분` 헤더 행**: 연령대 열 라벨 = `합계`, `18세`, `19세`, `20-24세`, `25-29세`, `30-34세`,
  `35-39세`, `40-49세`, `50-59세`, `60-69세`, `70-79세`, `80세이상`. 열 인덱스를 여기서 잡는다.
- **지역 블록**: col0 = 지역명(`전체`=시도 전체, 이후 각 구시군/선거구). 지역마다 col1 성별
  (`합계`/`남자`/`여자`) 3그룹, 각 그룹은 col2 지표(`선거인수`/`투표자수`/`투표율`) 3행. 즉
  지역당 9행 × 연령대 열.
- **값 포맷**: 콤마 포함 문자열(`710,801`) 또는 숫자 혼재. 투표율은 `69.4` 형태.

## 3. 정규화 레코드 (`internal/nec/turnout.go`, 신규)

**wide→long 피벗**: (지역, 성별, 연령대)마다 레코드 1건, 세 지표를 묶는다.

```go
type TurnoutRecord struct {
    Election    string  `json:"election"`    // 데이터셋명에서 (예: "제22대 국회의원선거 투표율 분석")
    Category    string  `json:"category"`    // 대괄호 태그 verbatim (예: "표본-일반", "표본")
    RegionLevel string  `json:"regionLevel"` // "구시군" | "선거구" (row0 제목에서)
    Sido        string  `json:"sido"`        // 시트명
    Region      string  `json:"region"`      // "전체" | "종로구" | ... (col0)
    Gender      string  `json:"gender"`      // "합계" | "남자" | "여자" (col1)
    AgeGroup    string  `json:"ageGroup"`    // "합계" | "18세" | "20-24세" | ... (구분 헤더 열)
    Electorate  int     `json:"electorate"`  // 선거인수 (원자료)
    Voters      int     `json:"voters"`      // 투표자수 (원자료)
    Rate        float64 `json:"rate"`        // 투표율 — 소스 제공 원자료 (우리 파생 아님)
}
```

- `func ParseTurnoutAnalysis(zipRaw []byte) ([]TurnoutRecord, error)` — ZIP 바이트를 받아 대상
  XLSX를 파싱. `archive/zip`으로 열고 각 `.xlsx` 엔트리를 excelize로 파싱. 앵커(구분 헤더 +
  전체/남/여 × 3지표)에 맞지 않는 시트/파일은 stderr 경고 후 skip(어떤 파일도 매칭 안 되면 에러).
- 콤마 제거는 기존 `atoiLoose`(results.go) 재사용.
- **`Election` 은 파서가 채우지 않는다** — zipRaw만으로는 알 수 없으므로, 호출부(CLI/ingest)가
  다운로드 파일명 또는 데이터셋명에서 채운다. 파서 반환 레코드의 `Election`은 빈 문자열이고,
  호출부가 일괄 세팅한다(단일 출처, 모호성 제거).

## 4. 컴포넌트

```
internal/nec/turnout.go   ParseTurnoutAnalysis(zipRaw) → []TurnoutRecord (앵커 기반, graceful-skip)
internal/store/schema.go  (수정) turnout 테이블 + v_turnout_derived 뷰 추가
internal/store/ingest.go  (수정) IngestTurnout(meta, []nec.TurnoutRecord) — dataset 단위 멱등
internal/mcpserver/ingest.go (수정) ingest_turnout tool
cmd/kvote/nec.go          (수정) nec turnout-analysis <pk> [--file zip]
cmd/kvote/db.go           (수정) kvote db ingest turnout <pk>
```

- ZIP 처리는 `turnout.go` 안에 격리(다운로드는 기존 `Download`가 zip을 그대로 반환 → 바이트를
  `ParseTurnoutAnalysis`에 전달). 새 네트워크 코드 없음.

## 5. DB 스키마 (`internal/store`)

```sql
CREATE TABLE turnout (
  id          INTEGER PRIMARY KEY,
  dataset_id  INTEGER NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
  election    TEXT, category TEXT, region_level TEXT, sido TEXT, region TEXT,
  gender      TEXT, age_group TEXT,
  electorate  INTEGER, voters INTEGER, rate REAL   -- rate = 소스 제공 원자료
);
CREATE INDEX idx_turnout_dataset ON turnout(dataset_id);

-- 표준 파생: 소스 보고 투표율(rate) 옆에 재계산값을 나란히. 정의 명시(중립).
CREATE VIEW v_turnout_derived AS
SELECT id, dataset_id, election, category, region_level, sido, region, gender, age_group,
       electorate, voters, rate AS rate_reported,
       CASE WHEN electorate > 0 THEN CAST(voters AS REAL)/electorate*100 END AS rate_computed
FROM turnout;
```

- `IngestTurnout(meta DatasetMeta, recs []nec.TurnoutRecord) (int64, error)`: `(source, public_data_pk)`
  기준 DELETE-then-INSERT(멱등), 단일 트랜잭션. v0.2.0 `IngestResults`/`IngestPolls`와 동일 패턴.
- **`schemaVersion`은 1 유지 (올리지 않는다)**: turnout 테이블/뷰는 순수 *추가* DDL이고 모두
  `CREATE ... IF NOT EXISTS`다. `migrate()`는 매 `Open`마다 전체 `SchemaSQL`을 재적용하므로,
  기존 v1 DB는 다음 `Open` 때 turnout 객체를 자동으로 얻는다(데이터 보존, 에러 없음). 버전을 2로
  올리면 `migrate()`의 `v != 0 && v != schemaVersion` 가드가 기존 v1 DB를 재생성 대상으로
  잘못 판정하므로, 하위호환 추가 변경에는 버전을 올리지 않는 것이 맞다.

## 6. MCP / CLI 표면

- MCP tool `ingest_turnout(pk)`: 키리스 다운로드(zip) → `ParseTurnoutAnalysis` → `IngestTurnout`.
  적재 요약(행수·election) 반환. 에러는 tool-level `errResult`.
- `kvote db ingest turnout <pk>`: 위와 동일 경로의 CLI 래퍼. `--db` 존중.
- `nec turnout-analysis <pk> [--file ZIP]`: 정규화 레코드를 `-f json|jsonl|table`로 출력
  (다운로드 없이 로컬 zip 파싱은 `--file`). 대용량이라 jsonl 권장. DB 적재 안 함(순수 뷰).
- `kvote db query`/MCP `query`로 turnout·v_turnout_derived를 다른 데이터셋과 조인 질의.

## 7. 에러 처리

- ZIP이 아니거나(단일 xlsx 등) 대상 시트가 하나도 없으면 명확한 에러 + "원본은 `nec pull`" 안내.
- 개별 시트 파싱 실패는 stderr 경고 후 skip(다른 시트 계속) — `xlsx.go` 정책과 일치.
- `electorate = 0`인 행의 `rate_computed`는 NULL(NaN 방지). 소스 `rate`는 그대로 저장.
- 부분 적재 실패 시 트랜잭션 롤백(전부/전무).

## 8. 테스트 (네트워크 없음)

- `internal/nec`: 축소 XLSX를 zip으로 묶은 `testdata` 픽스처로 `ParseTurnoutAnalysis` 검증 —
  (지역×성별×연령) 피벗 정확, 콤마 파싱, `Category`/`RegionLevel` 추출, 앵커 불일치 시트 skip,
  대상 없으면 에러.
- `internal/store`: `IngestTurnout` 멱등(재적재 중복 없음)·트랜잭션 롤백, `v_turnout_derived`의
  `rate_computed` 계산·electorate 0 가드.
- `internal/mcpserver`: zip 픽스처를 서빙하는 httptest + `WithBaseURL`로 `ingest_turnout` 왕복,
  이후 `query`로 행수 확인.

## 9. 하지 않는 것 (YAGNI)

- **사전투표·재외투표 인구통계 시트**: 레이아웃이 종합/비교표로 상이 — 별도 사이클(후속 P3.1).
- **시간대별(hourly) 투표율**: 이 데이터셋에 없음(선관위 별도 공개물). 범위 밖.
- **분포(선거인/투표자)·종합 시트**: 성별·연령 교차표에 집중, 나머지 skip.
- **마이그레이션 체인**: 없음. 이번은 순수 추가 DDL이라 `schemaVersion` 유지 + `IF NOT EXISTS`로
  기존 DB에 무해하게 확장(§5). 향후 파괴적 스키마 변경이 필요할 때 별도 설계.
- **PDF만 공개된 구(舊) 선거**(제18대 대선 등): XLSX만 대상, PDF는 `nec pull`로 원본 안내.
