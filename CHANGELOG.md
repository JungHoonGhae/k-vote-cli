# Changelog

`kvote` 사용자 관점의 변경 이력입니다. 각 버전에서 "어떤 선거 데이터를 어떻게 받을 수 있게 됐는지"를 정리합니다.

형식은 [Keep a Changelog](https://keepachangelog.com/ko/1.1.0/)를 따르고, 버전은 [SemVer](https://semver.org/lang/ko/)를 따릅니다.

## [Unreleased]

## [0.1.0] - 2026-06-28

첫 공개 릴리즈. 흩어진 한국 선거 공개 데이터(개표결과·여론조사)를 **키 없이** 한 명령으로 검색·다운로드·정규화합니다.

### NEC — 개표결과 (중앙선거관리위원회 → data.go.kr 파일 데이터, 키리스)
- **`nec corpus`** — 역대 핵심 선거 9종(대선·총선·비례·지방)의 개표결과를 동시 다운로드합니다. `--normalize` 로 투표구별 정규화 JSONL까지 한 번에.
- **`nec datasets`** — 선관위 공개 파일 데이터(개표결과·투표율 등)를 검색합니다.
- **`nec latest`** — 선거종류의 최신 회차 데이터셋을 두 소스에서 자동 해석합니다.
- **`nec pull`** — 파일 데이터(CSV/XLSX) 원본을 그대로 받습니다.
- **`nec results`** — 개표결과 CSV(EUC-KR)를 투표구별 정규화 레코드로 변환합니다. 원자료를 보존하면서 정의가 명시된 표준 파생값(투표율·득표율·유효표)만 더합니다.

### NESDC — 여론조사 (중앙선거여론조사심의위원회, 키리스)
- **`nesdc sync`** — 기간/조건 전체를 페이지네이션하며 JSONL로 일괄 수집합니다. "이번주 여론조사 전수"가 한 명령.
- **`nesdc bulk`** — 주차별 누적 마스터 엑셀을 정규화된 여론조사 레코드로 출력합니다.
- **`nesdc show` / `nesdc pull`** — 단건 상세 메타 조회 및 첨부(통계표·설문지) 다운로드.

### data.go.kr OpenAPI (실험적 — 인증키 필요)
- **`nec turnout` / `nec winners` / `nec elections`** — 투표율·당선인·선거코드를 data.go.kr OpenAPI로 조회합니다(`KVOTE_DATAGOKR_KEY`).
- **`api login` / `list` / `apply` / `config`** — 브라우저 세션으로 data.go.kr에 로그인해, 필요한 OpenAPI 활용신청을 확인·신청합니다. 키 입력은 사용자가 브라우저에서 직접 하며 kvote는 비밀번호를 보지 않습니다.

### 도구
- **`kvote doctor`** — 킬러 경로(검색→다운로드→정규화)를 라이브로 점검하는 스모크 테스트.
- 모든 출력은 `--format json | jsonl | table`. 한글 폭 보정 테이블 내장.

### 설계 원칙
- **키리스**: 핵심 기능(개표결과·여론조사)은 API 키 발급 없이 동작합니다.
- **중립**: kvote는 어떤 분석적 입장도 취하지 않습니다. 원자료를 완전 보존하고, 정의가 명시된 재현 가능한 표준 파생값만 더합니다. 플래그·점수·"이상치" 판단은 제공하지 않습니다.
- **예의 있는 수집**: 요청 간 `--delay`(기본 700ms)를 보장하고, 탐지 회피로 차단된 소스를 우회하지 않습니다.

### 배포
- macOS·Linux·Windows (amd64·arm64) 바이너리. Homebrew tap(`JungHoonGhae/k-vote-cli`) 제공.

[Unreleased]: https://github.com/JungHoonGhae/k-vote-cli/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/JungHoonGhae/k-vote-cli/releases/tag/v0.1.0
