# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 개요

`kvote` 는 한국 선거 공개 데이터의 **접근성을 높이는** 비공식 Go CLI 입니다. **멀티-프로바이더**
구조로 NESDC(중앙선거여론조사심의위원회, 여론조사)와 NEC(중앙선거관리위원회 → data.go.kr,
개표결과) 를 키 없이 검색·다운로드·정규화합니다. **API 키 발급이 필요 없는** 키리스 접근이
핵심 설계 원칙입니다.

### 취지 + 중립성 원칙 (코드 작성 시 반드시 지킬 것)

- **취지**: 선관위 투명성에 대한 사회적 의구심 속에서, 흩어진(robots 차단·JSF·PDF) 공개
  선거 데이터를 누구나·AI 에이전트가 한 번의 명령으로 구조화해 받을 수 있게 — 접근성 인프라.
- **중립성(타협 불가)**: kvote 는 **어떤 분석적 입장도 취하지 않는다.** 데이터 접근성만
  높이고, 무엇이 정상·비정상·수상한지 판단하지 않는다.
  - 제공: 분석 파라미터·차원을 광범위·중립적으로 — 원자료 완전 보존 + *정의가 명시된 재현
    가능한 표준 파생값*(비율·합계·집계).
  - 금지: 플래그·점수·순위·"이상치"·"검증 결과"·해석. "수상함"을 내장한 휴리스틱 금지.
    판단은 소비자(사람/AI 에이전트)의 몫.
- 로드맵·Phase 설계: `docs/superpowers/specs/2026-06-21-kvote-coverage-roadmap-design.md`.

## 명령어

```bash
make build      # bin/kvote 빌드 (ldflags 로 버전 주입)
make test       # go test ./...  — 네트워크 없이 testdata 픽스처로 파서 검증
make fmt        # gofmt -w ./cmd ./internal
make tidy       # go mod tidy
go vet ./...

# 단일 테스트
go test ./internal/nesdc -run TestDetailResults -v
```

라이브 동작 확인:

```bash
./bin/kvote nesdc list -f table          # 여론조사 결과 목록
./bin/kvote nesdc show <nttId>           # 상세 메타
./bin/kvote nesdc pull <nttId> -o /tmp/x # 첨부 다운로드
```

## 아키텍처

```
cmd/kvote/          CLI (cobra). provider 별로 명령 그룹 분리.
  root.go           전역 플래그(--format/--delay/--base-url), 클라이언트 빌더
  nesdc.go          nesdc 명령 그룹 + 렌더링 헬퍼(renderList/renderDetail/renderAgencies)
  nec.go            nec 명령 그룹 (datasets/pull)
internal/nec/       NEC provider — data.go.kr 공개 파일 데이터 클라이언트 (package nec)
  client.go         rate-limited HTTP + getDoc
  datasets.go       선관위 파일 데이터 검색 파서 (selectDataSetList.do, dt 포맷/제목 분리)
  download.go       uddi 조회 → selectFileDataDownload.do(atchFileId) → fileDownload.do
  results.go        개표결과 CSV(EUC-KR) long-format → 투표구별 ResultRecord 정규화
  election.go       CSV·XLSX 공통 스키마(ElectionResult) + voteType/aggregate 파생
  xlsx.go           XLSX 멀티시트 wide→long 파서 (앵커 라벨 기반, 선거별 손매핑 없음)
  aggregate.go      투표구 레코드 → 다단계 집계(AggLevel) + 파생값(투표율·득표율·유효표). 중립.
  openportal.go     data.nec.go.kr 개방포털 소스 (datasets/files/download). robots 허용·키리스.
  filename.go       Content-Disposition 파일명 인코딩 복구
internal/nesdc/     NESDC provider — HTML 스크래핑 클라이언트 (package nesdc)
  client.go         rate-limited HTTP + goquery 파서 진입점(getDoc)
  board.go          게시판 레지스트리 (bbsId+menuNo 로 파라미터화)
  list.go           목록 파서 (.row.th 헤더 → .row.tr 행) + 검색/기간 필터
  filters.go        필터 코드 매핑(searchCnd/searchTime) + 선거구분(elections) 스크래퍼
  detail.go         view.do 상세 파서 (메타 테이블 + 첨부; onclick·href 두 형태)
  composition.go    Detail.Fields → 표본 구성 교차표(SampleComposition) 파생. 중립.
  download.go       FileDown.do 다운로드 + 파일명 인코딩 복구
  bulk.go           data 게시판 누적 마스터 엑셀(excelize) → 정규화 PollRecord
  agency.go         onvy 조사기관 등록/취소 현황 파서
internal/output/    json / jsonl / table 렌더러 (한글 폭 보정)
internal/version/   ldflags 주입 버전 메타
```

### 핵심 설계 포인트 (코드 읽기 전 알아둘 것)

- **모든 NESDC 게시판은 동일한 eGovFrame 엔진**: `bbs/{bbsId}/list.do` · `view.do` ·
  `cmm/fms/FileDown.do`. 새 게시판 추가는 `board.go` 의 `boards` 맵에 한 줄 추가로 끝.
- **목록 마크업은 `<table>` 이 아님**: 헤더는 `.row.th`, 데이터는 `.row.tr`(`<a>` 또는 `<p>`),
  셀은 `.col`. `nttId` 는 행/중첩 앵커의 href 에서 정규식으로 추출. 헤더 라벨을 그대로 JSON 키로
  사용하므로 게시판마다 컬럼이 자기 설명적.
- **첨부 다운로드의 atchFileId/fileSn 는 이미 percent-encoded**. `DownloadURL` 은 재인코딩하지
  않고 문자열 결합(중요 — `url.Values.Encode()` 쓰면 `%` 가 이중 인코딩됨).
- **Content-Disposition 파일명 인코딩이 두 형태**: percent-encoded UTF-8(+공백) 또는
  raw UTF-8 가 Latin-1 로 잘못 읽힌 형태. `decodeFilename`/`fixLatin1UTF8` 가 둘 다 복구.
- **상세 메타는 lossless**: `Detail.Fields` 는 모든 th/td 행을 순서대로 보존(176행 규모).
  `Detail.Summary` 는 단일 라벨 행 중 분석 핵심 스칼라만 추림(`summaryLabels`). 일부 첨부는
  공표일시 24/48시간 후 공개라 그 전엔 다운로드가 "embargoed" 에러로 표면화됨.
- **첨부 앵커는 게시판마다 두 형태**: results 계열은 `onclick="view('id','sn','bbs','key')"`,
  data/notice 계열은 `<a href=".../FileDown.do?atchFileId=...&fileSn=...&bbsId=...">`(bbsKey 없음).
  `parseAttachments` 가 둘 다 처리하며, href 쿼리는 `parseRawQuery` 로 **percent-decode 없이** 추출
  (`url.Values` 쓰면 `%` 가 풀려 다운로드가 깨짐).
- **기간 필터는 `searchTime` 동반 필수**: 포털은 `sdate`/`edate` 만 보내면 **조용히 무시**한다.
  `List` 는 `From/To` 가 있으면 `searchTime` 을 자동 세팅(기본 `1`=등록일). `filters.go` 의
  `SearchField`/`DateField` 가 친숙한 이름(agency/registered…)을 raw 코드로 매핑.
- **data 게시판 누적 엑셀**: 매 주차 글에 같은 마스터 `.xlsx`(2023.10.30~ 전체 누적)가 재첨부됨.
  `LatestBulkXlsx` 가 최신 글에서 `.xlsx` 첨부를 찾고, `ParseBulkXlsx` 가 시트(기간)별 2행 헤더
  (고정 메타 10열 + `정당지지율` 아래 동적 정당 열)를 `PollRecord` 로 정규화.
- **rate limit**: `Client.throttle()` 이 요청 간 `--delay`(기본 700ms) 보장. 예의 있는 수집 원칙.

### 테스트 전략

`internal/nesdc/nesdc_test.go`·`internal/nec/nec_test.go` 는 `httptest.Server` 로 픽스처를
서빙하고 `WithBaseURL` 로 클라이언트를 연결해 **네트워크 없이** 전체 경로(URL 빌드 → 파싱 →
다운로드)를 검증합니다. nesdc 는 `testdata/*.html`, nec 은 테스트 내 인라인 HTML/JSON 응답.
마크업이 바뀌면 픽스처를 최신본으로 교체하세요.

## NEC provider (data.go.kr 키리스 파일 데이터)

- **info.nec.go.kr(선거통계시스템)은 직접 다루지 않는다**: robots.txt `Disallow: /` 에 더해,
  콘텐츠 페이지가 헤드리스 브라우저 세션 안에서도 "비정상적인 접근"으로 거부됨(JSF 상태기반).
  탐지 회피로 우회하지 않는다 — 대신 선관위의 **공식 배포 채널**을 쓴다.
- **소스 = data.go.kr 파일 데이터**: 선관위가 개표결과·투표율을 CSV/XLSX 파일셋으로 공개.
  파일 다운로드 자체는 **API 키 불필요**(키 발급형 OpenAPI 와 별개).
- **다운로드 3단계**(`download.go`): ① `/data/{pk}/fileData.do` 상세에서 `fn_fileDataDown(pk,
  'uddi:…')` 의 publicDataDetailPk 추출 → ② `selectFileDataDownload.do` 가 `atchFileId`·파일명
  JSON 반환 → ③ `/cmm/cmm/fileDownload.do?atchFileId=&fileDetailSn=&dataNm=` 가 실제 파일 스트림.
- **개표결과 CSV 정규화**(`results.go`): CP949(EUC-KR) long-format(시도/선거구/읍면동/투표구/
  후보자/득표수)을 투표구별 `ResultRecord`(선거인수·투표수·무효·기권 + 후보 득표)로 묶는다.
  **블록 구분자는 `선거인수` 행** — (시도,선거구,읍면동,투표구) 튜플은 통합 선거구·분할 사전투표로
  중복돼 키 기준 그룹핑하면 동명 투표구가 병합된다(434건 실측). 후보 행은 `정당 이름`을 첫 공백
  기준으로 분리(무소속 포함). `nec pull` 은 원본, `nec results` 는 정규화(--file 로 로컬 파싱).

## 컨벤션

- 출처별 provider 는 `internal/<provider>` 패키지로 격리, `cmd/kvote/<provider>.go` 가 명령 노출.
- 새 출력 타입은 `internal/output` 의 `WriteJSON`/`WriteJSONL`/`WriteTable` 재사용.
- 한글 정렬이 필요한 표는 `output.WriteTable`(CJK 더블폭 계산 내장)만 사용.
