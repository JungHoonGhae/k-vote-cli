<div align="center">
  <h1>kvote</h1>
  <p><strong>한국 선거 관련 공개 데이터를 수집하는 비공식 CLI.</strong></p>
  <p>API 키 발급 없이 — 여론조사 결과·조사기관 현황을 목록/상세/첨부파일/일괄(JSONL)로 가져옵니다.</p>
</div>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8.svg" alt="Go" />
  <img src="https://img.shields.io/badge/output-json%20%7C%20jsonl%20%7C%20table-informational" alt="output formats" />
  <img src="https://img.shields.io/badge/license-MIT-yellow.svg" alt="MIT" />
</p>

> [!WARNING]
> 비공식 도구입니다. 공개 포털의 HTML 구조에 의존하므로 사이트 개편 시 동작이 깨질 수 있습니다.
> 수집 대상은 **법적 공개 의무가 있는 공공 데이터**(선거여론조사기준에 따른 등록·공개 자료)이며,
> 기본 rate limit 을 적용해 예의 있게 수집합니다. 본인 책임 하에 사용하세요.

## 데이터 출처 (provider)

| provider | 사이트 | 내용 | 상태 |
|---|---|---|---|
| `nesdc` | 중앙선거여론조사심의위원회 (nesdc.go.kr) | 여론조사 결과·조사기관 현황 | ✅ |
| `nec` | 중앙선거관리위원회 (nec.go.kr) | 선거 통계 (개표·투표율 등) | 🚧 준비 중 |

`nesdc.go.kr` 은 공식 API 가 없어 스크래핑이 유일한 프로그램적 접근입니다.
`nec` 는 통계시스템(info.nec.go.kr)이 robots.txt 로 전면 크롤링을 금지하고 공식 API 는 키 발급이
필요하므로, **키 없이 받을 수 있는 공개 데이터 파일**(data.nec.go.kr / data.go.kr 파일셋)을
우선하도록 설계 중입니다. (`kvote nec roadmap`)

## 설치

```bash
git clone https://github.com/JungHoonGhae/kvote
cd kvote
make build        # -> bin/kvote
# 또는
make install      # $GOBIN 에 설치
```

## Quick Start

```bash
# 여론조사 결과 최신 목록 (표 형식)
kvote nesdc list -f table

# 특정 게시판 / 검색 / 기간
kvote nesdc list data --from 2026-01-01 --to 2026-06-30 -f table
kvote nesdc list -q 리얼미터 -f table

# 단건 상세 메타데이터 (기관·방식·표본·응답률·표본오차·공표일시 + 교차표)
kvote nesdc show 19366

# 첨부파일(통계표·설문지 PDF) 다운로드
kvote nesdc pull 19366 -o ./downloads

# 조사기관 등록현황 / 취소현황
kvote nesdc agencies -f table
kvote nesdc agencies --cancelled -f table

# 전체 일괄 수집 → JSONL (대규모 분석용)
kvote nesdc sync --from 2026-01-01 > surveys.jsonl
kvote nesdc sync --max-pages 5 --pull -o ./archive   # 메타 + 첨부 동시 수집
```

## 게시판 (nesdc)

`kvote nesdc boards` 로 확인. 모두 동일한 엔진으로 처리됩니다.

| name | 내용 |
|---|---|
| `results` | 여론조사결과 보기 (상세 메타 + PDF) — 기본값 |
| `data` | 여론조사결과 주요 데이터 (주차별 벌크) |
| `notices` | 공지사항 |
| `library` | 자료실 |
| `actions` | 선거여론조사기관 조치현황 |
| `violations` | 유형별 위반사례 |

## 출력 형식

| `-f` | 용도 |
|---|---|
| `json` | 기본. 사람이 읽거나 `jq` 로 가공 |
| `jsonl` | 한 줄에 한 레코드. 대량 수집/스트리밍 |
| `table` | 터미널에서 바로 보기 (한글 폭 정렬) |

## 전역 플래그

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `-f, --format` | `json` | 출력 형식 |
| `--delay` | `700ms` | 요청 간 최소 간격 (rate limit) |
| `--base-url` | — | 포털 base URL 재정의 (테스트용) |

## 개발

```bash
make build    # 빌드
make test     # 테스트 (네트워크 불필요 — testdata 픽스처 사용)
make fmt      # gofmt
go vet ./...
```

자세한 내부 구조는 [CLAUDE.md](CLAUDE.md) 참고.

## 라이선스

MIT
