# kvote P5 설계 — `kvote mcp`: 통합 로컬 데이터셋(SQLite) + MCP 질의 인터페이스

작성일: 2026-07-02
상태: 승인됨 (브레인스토밍 완료, 구현 계획 대기)
선행: `2026-06-21-kvote-coverage-roadmap-design.md` 의 P5 항목 구체화

## 1. 목표

kvote를 모르는 AI 에이전트도 **MCP 연결 하나**로 한국 선거 공개 데이터의
탐색 → 키리스 수집 → SQL 질의를 끝낼 수 있게 한다.

- 주 사용자 = AI 에이전트 (제품 전략과 정합). 사람용은 디버깅 수준의 얇은 CLI 래퍼만.
- 첫 버전 적재 범위: **개표결과**(`nec results` 정규화 경로의 `ElectionResult`) +
  **여론조사**(`nesdc bulk` 의 `PollRecord`). 투표율(P3)은 별도 사이클.
- 중립성 원칙 그대로 적용: 원자료 완전 보존, 파생값은 *정의가 DDL(뷰 SQL)에 명시된
  재현 가능한 표준 변환*만. 플래그·점수·해석 없음. SQL 패스스루 자체가 "판단은
  소비자의 몫" 원칙의 구현체다.

## 2. 접근안 결정 기록

| 결정 | 선택 | 기각안과 이유 |
|---|---|---|
| MCP 역할 범위 | 수집+질의 통합 | 질의 전용(수집을 CLI에 의존 — 에이전트 온보딩 마찰), 얇은 CLI 래퍼(질의 능력 부재) |
| DB 엔진 | SQLite (modernc.org/sqlite, 순수 Go) | DuckDB(cgo·배포 복잡, JSONL+duckdb 기존 경로와 중복), 무DB(유연성 제한) |
| tool 표면 | SQL 패스스루 + 최소 수집/탐색 tool | 구조화 tool 세트(슬라이스 선별 = 중립성과 긴장, 유지보수 부담) |
| 적재 범위 | 개표 + 여론조사 | 개표만(교차 질의 불가), 전수+P3(한 사이클 과대) |

## 3. 아키텍처

```
internal/store/      SQLite 스키마·적재·질의 (modernc.org/sqlite, cgo 없음)
  schema.go          DDL + 표준 파생 뷰 (뷰 정의 자체가 파생값 문서)
  ingest.go          []ElectionResult → datasets/results/candidates,
                     []PollRecord → polls/party_support (dataset 단위 멱등 교체)
  query.go           read-only 질의 실행 (행수 상한, 컬럼+행 구조 반환)
internal/mcpserver/  MCP stdio 서버 (공식 modelcontextprotocol/go-sdk)
  server.go          tool·resource 등록 및 핸들러 — nec/nesdc 클라이언트와 store 조립만
cmd/kvote/mcp.go     `kvote mcp` (stdio serve)
cmd/kvote/db.go      `kvote db ingest results <pk>` / `db ingest polls` / `db query <sql>`
                     — store 를 그대로 감싸는 사람/디버깅용 얇은 래퍼
```

- 다운로드·파싱은 전부 기존 코드 재사용: `internal/nec`(openportal/download/results/xlsx),
  `internal/nesdc`(bulk). `mcpserver` 와 `store` 는 새 파싱 로직을 갖지 않는다.
- `store` 는 네트워크 무관 순수 패키지 — `Ingest*` 는 이미 파싱된 레코드 슬라이스를 받는다.

## 4. DB 스키마

기본 경로: `os.UserConfigDir()` 계열 관례에 따라 darwin `~/Library/Application
Support/kvote/kvote.db`, linux `~/.local/share/kvote/kvote.db`. 전역 `--db` 플래그로
오버라이드 (mcp·db 명령 공통).

### 4.1 테이블 (원자료 — 항상 그대로 보존)

- `datasets(id, source, public_data_pk, name, election_name, ingested_at, row_count)`
  — 출처·계보(provenance) 메타. **재적재 = 해당 dataset 의 하위 행 전체 삭제 후 재삽입**(멱등).
- `results(id, dataset_id, sido, sgg, town, booth, vote_type, electorate, votes,
  invalid, abstention)` — 투표구 단위 원자료. `vote_type` 은 P1의 구조 라벨
  (본투표/관내사전/관외사전/거소선상) 그대로.
- `candidates(id, result_id, party, name, votes)` — long format.
- `polls(id, dataset_id, …)` — `PollRecord` 의 고정 메타 10열(기간·기관·의뢰처·방법·
  표본 등)을 1:1 컬럼으로. 정확한 컬럼명은 구현 시 `PollRecord` 필드에서 그대로 도출.
- `party_support(id, poll_id, party, pct)` — 동적 정당 열의 long format.

### 4.2 뷰 (표준 파생 — 정의는 뷰 SQL 그 자체)

- `v_results_derived` — 유효투표수 = votes − invalid, 투표율 = votes/electorate
  (electorate 0 이면 NULL), 후보 득표율 = candidate votes / 유효투표수.
- `v_agg_town / v_agg_sgg / v_agg_sido / v_agg_national` — 기존 `internal/nec/aggregate.go`
  의 `AggLevel` 정의를 SQL 로 동일 이식. 각 뷰는 `vote_type` 차원을 포함(GROUP BY 에
  vote_type — 전체 합은 소비자가 SUM). sgg 이하만 후보 합산, sido/national 은 지표만
  — aggregate.go 와 같은 한계·이유를 뷰 주석에 명시.

Go 코드(`aggregate.go`)와 SQL 뷰가 같은 정의의 두 구현이 되므로, 테스트에서 동일
픽스처에 대해 **두 경로의 결과 일치를 검증**해 정의 드리프트를 막는다.

## 5. MCP 표면

stdio transport. tool 5개 + resource 1개.

| tool | 입력 | 동작 |
|---|---|---|
| `search_datasets` | keyword | data.go.kr/개방포털 파일데이터 검색 (기존 `nec datasets` 경로) |
| `list_elections` | (없음/필터) | 선거 목록 (기존 `nec elections` 경로) |
| `ingest_results` | pk | 키리스 다운로드 → CSV/XLSX 정규화 → 적재. 적재 요약(행수·선거명) 반환 |
| `ingest_polls` | (없음) | NESDC 누적 마스터 엑셀 → 적재. 요약 반환 |
| `query` | sql, limit(기본 1000) | **read-only** SQL 실행. `{columns, rows, truncated}` 반환 |

- resource `kvote://schema` — 테이블·뷰 DDL 전문 + 파생값 정의 설명. 에이전트가
  query 전에 읽는 진입점. tool description 에도 "먼저 schema resource 를 읽어라" 명시.
- rate limit·`--delay` 는 기존 클라이언트 그대로 (ingest tool 이 내부적으로 준수).

## 6. 에러 처리

- `query` 의 쓰기 차단: 질의용 연결을 SQLite **read-only 모드**로 별도로 연다 —
  쓰기 SQL 은 엔진 레벨에서 거부되므로 SQL 파싱·필터링 불필요.
- `limit` 초과분은 자르고 `truncated: true` 로 표기 (조용한 손실 방지).
- ingest 중 파싱 불가(비례대표 XLSX 레이아웃 등)는 기존 에러를 그대로 표면화하고
  "원본은 `nec pull`" 안내 유지. 부분 적재 실패 시 트랜잭션 롤백 — dataset 은
  전부 들어가거나 전혀 안 들어가거나.
- DB 파일이 없으면 첫 사용 시 자동 생성(스키마 마이그레이션은 `PRAGMA user_version`).

## 7. 테스트 (전부 네트워크 없음)

- `internal/store`: in-memory SQLite(`:memory:`) + 픽스처 레코드.
  - 적재 멱등성(재적재 시 중복 없음), 트랜잭션 롤백.
  - **aggregate.go ↔ SQL 뷰 동치성**: 같은 픽스처로 두 경로 결과 비교.
  - read-only 연결에서 INSERT/UPDATE 거부 확인. limit·truncated 동작.
- `internal/mcpserver`: go-sdk 의 in-process transport 로 tool 호출 왕복 검증.
  ingest tool 은 기존 `httptest` 픽스처 서버 + `WithBaseURL` 재사용.
- `cmd` 래퍼는 얇으므로 스모크 수준.

## 8. 하지 않는 것 (YAGNI)

- 구조화 질의 tool(get_results 류) — SQL 패스스루로 충분, 중립성 긴장 회피.
- HTTP/SSE transport — stdio 만. 필요해지면 별도 사이클.
- 투표율(P3)·여론조사 교차표(SampleComposition) 적재 — 다음 버전.
- DB 스키마의 하위 호환 마이그레이션 체인 — v1 은 버전 불일치 시 재생성 안내로 충분.
