# Phase: data.nec.go.kr (국가선거정보 개방포털) 소스 추가 설계

작성일: 2026-06-22
상태: 설계 (착수)
상위: 로드맵 — NEC 신선·완전 데이터 접근성

## 1. 동기

선관위 **국가선거정보 개방포털 `data.nec.go.kr`** 은 robots `Allow: /` · 무료 · 이용허락
제한 없음 · **키 불필요**의 합법 소스로, 다음이 실측 확인됐다:

- `file-download.do?attachFileId=N` 직접 다운로드 (제21대 대선 개표결과 XLSX 5.6MB 수신 ✓)
- 개표결과 + **투표율·당선인·후보자·유권자의식조사** (data.go.kr 엔 거의 없는 종류)
- **신선도**: 제8회 지방선거(2022.06.01) → 개방 2022-06-22 (선거 후 21일). data.go.kr 보다 빠름.
- 파일은 **XLSX** → 기존 `ParseResultsXLSX`(P2)가 그대로 처리.

info.nec.go.kr(차단)을 우회하지 않고도 가장 빠른 합법 경로다. 제9회(2026.06.03)는 전례상
임박 — provider 를 만들어 두면 올라오는 즉시 픽업된다.

## 2. 엔드포인트 (실측)

- 카탈로그: `GET /open-data/data-list.do?keyword=<kw>&openDataType=ALL` → `dataId` 보유 목록.
  페이지당 6건(페이징 있음). 키워드 검색 지원.
- 상세: `GET /open-data/file.do?dataId=N` → 제목·메타 + 첨부 `file-download.do?attachFileId=M` 링크들
  (회차/포맷별 여러 개) + data.go.kr 상호링크.
- 다운로드: `GET /file-download.do?attachFileId=M` → 파일 스트림. `Content-Disposition`(RFC5987 +
  percent-UTF8) 파일명. **HTTP**(https 는 미동작) — baseURL `http://data.nec.go.kr`.
- 세션: 첫 요청에 `JSESSIONID` 쿠키 발급 — 쿠키 유지(`http.Client` jar 또는 무상태로도 동작 확인).

## 3. 설계 — 기존 `nec` 패키지에 2번째 소스

`internal/nec` 는 현재 data.go.kr 전용. **소스 개념을 도입**해 두 백엔드를 한 패키지에서.

### 3.1 타입

```go
// internal/nec/source.go
type Source string
const (
	SourceDataGoKr   Source = "datagokr"   // 기존 data.go.kr (기본)
	SourceOpenPortal Source = "openportal" // data.nec.go.kr 개방포털
)
```

`Client` 에 소스별 baseURL 을 두거나, 개방포털 전용 메서드를 추가한다. **최소 변경**을 위해
개방포털은 별도 메서드 군으로 분리(기존 data.go.kr 경로 무수정 → 하위호환):

```go
// internal/nec/openportal.go
const OpenPortalBaseURL = "http://data.nec.go.kr"

type OPDataset struct {
	DataID string `json:"dataId"`
	Title  string `json:"title"`
}
type OPFile struct {
	AttachFileID string `json:"attachFileId"`
	Name         string `json:"name"`   // Content-Disposition 또는 링크 텍스트
}

func (c *Client) OpenPortalDatasets(ctx, keyword string) ([]OPDataset, error)         // data-list.do 파싱
func (c *Client) OpenPortalFiles(ctx, dataID string) ([]OPFile, error)                // file.do?dataId 상세 파싱
func (c *Client) OpenPortalDownload(ctx, attachFileID, destDir string) (string, error)// file-download.do 스트림
```

개방포털 요청은 `OpenPortalBaseURL` 로 별도 `rawGet`(기존 `rawGet` 이 `c.baseURL` 고정이면
baseURL 인자를 받는 내부 헬퍼로 소폭 일반화). 파일명 복구는 기존 `filenameFromCD` 재사용.

### 3.2 CLI (`cmd/kvote/nec.go`)

`--source` 플래그(기본 `datagokr`)를 datasets/pull/results 에 추가:

- `nec datasets [-q] --source openportal` → `OpenPortalDatasets` → publicDataPk 대신 `dataId` 표기.
- `nec pull <dataId> --source openportal [-o]` → `OpenPortalFiles` 의 **XLSX 첨부**(여러 개면 최신/첫
  XLSX, 또는 `--attach <id>` 로 지정) → `OpenPortalDownload`.
- `nec results <dataId> --source openportal` → 다운로드(임시) → 확장자가 xlsx 면 `ParseResultsXLSX`,
  csv 면 `ParseResults` → 기존 렌더. **파서 재사용** 이 핵심.

기본(`datagokr`) 경로는 전부 그대로. 소스는 직교적 백엔드 선택일 뿐.

## 4. 에러 처리

- 개방포털 상세에 XLSX 가 여러 개(회차별/형식별): 기본은 첫 XLSX, 모호하면 `nec pull` 로 목록을
  stderr 안내. `nec results` 는 단일 파일 가정 — 여러 개면 첫 XLSX + 경고.
- https 접근 불가 → baseURL 고정 http. (포털 자체가 http.)
- 카탈로그 페이징: v1 은 첫 페이지(6건) + keyword 검색. 전체 순회는 후속(YAGNI).

## 5. 테스트

- `internal/nec/openportal_test.go`: `httptest` 로 data-list.do / file.do / file-download.do
  픽스처(인라인 HTML + 바이트) 서빙, `OpenPortalBaseURL` 을 테스트 서버로 오버라이드.
  - `TestOpenPortalDatasets`: dataId·title 파싱.
  - `TestOpenPortalFiles`: file.do 상세에서 attachFileId 추출.
  - `TestOpenPortalDownload`: file-download.do → 파일 + CD 파일명 복구.
- results 경로는 기존 `ParseResultsXLSX` 테스트가 이미 커버(파서 재사용).

## 6. 비범위

- 개방포털 OpenAPI(`/open-data/api.do`, 키 필요) · LOD/SPARQL(`/lod/sparql`) — 후속.
- 투표율·당선인·후보자 *정규화 파서* — 이번엔 **다운로드까지**(파일 확보). 개표결과만 기존 파서로
  정규화. 투표율/당선인 정규화는 별도 phase.
- 중립성: 어떤 판단도 없음. 접근 경로만 추가.
