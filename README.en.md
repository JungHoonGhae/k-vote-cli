<p align="right"><a href="README.md">한국어</a> · <strong>English</strong></p>

<div align="center">
  <h1>k-vote-cli</h1>
  <p><strong>Election results anyone can pull for themselves — Korea's public election data in one command.</strong></p>
  <p>Vote counts & opinion polls, with no special access or sign-up. A human or an AI gets them with the same command (<code>kvote</code>) and reads them right away.</p>
  <p><sub><strong>Download the core historical results at once + tidy them per precinct</strong> · full opinion-poll collection · turnout & winners · automatic API-key issuance — <a href="#features">full feature table ↓</a></sub></p>
</div>

<p align="center">
  <a href="LICENSE"><img src="docs/badges/license.svg" height="44" alt="MIT License" /></a>
  &nbsp;<img src="docs/badges/keyless.svg" height="44" alt="keyless core" />
  &nbsp;<img src="docs/badges/go.svg" height="44" alt="Built with Go" />
  &nbsp;<img src="docs/badges/agents.svg" height="44" alt="Works with Claude · Codex · jq · duckdb" />
  &nbsp;<img src="docs/badges/output.svg" height="44" alt="Output: json · jsonl · table" />
</p>

<p align="center">
  <a href="#quick-start"><strong>Quick Start</strong></a> ·
  <a href="#features"><strong>Features</strong></a> ·
  <a href="#install"><strong>Install</strong></a> ·
  <a href="#what-you-can-verify"><strong>Verify</strong></a> ·
  <a href="CLAUDE.md"><strong>Internals</strong></a>
</p>

<p align="center">
  <img src="docs/k-vote-cli.gif" alt="k-vote-cli demo — search results → per-precinct structured JSON → live health check" width="760" />
</p>

> [!WARNING]
> Unofficial tool. It depends on the public portals' HTML structure, so a site redesign may break it.
> It only collects **legally-mandated public data** (registered/published material under election-survey rules),
> with a default rate limit to scrape politely. Use at your own risk.

## The problem it solves

Korean election results and opinion polls are public. But getting them programmatically hits walls:

- a full robots block (info.nec.go.kr), JSF portals, PDFs
- EUC-KR encoding, a different XLSX layout per sheet, duplicated precincts
- the access-application + API-key issuance flow on data.go.kr

`k-vote-cli` removes that friction with **one command**. It turns scattered public data — keylessly — into a
structured form an AI agent can query directly.

> **Neutrality.** k-vote-cli preserves the raw data as-is and computes only standard, clearly-defined derived
> values (turnout, vote share, aggregates). It makes **no judgment** about what is normal or abnormal —
> analysis and interpretation are entirely up to whoever receives the data (human or AI).

## Before → After

| Data | Before | k-vote-cli |
|---|---|---|
| **Vote counts** (per precinct) | info.nec.go.kr blocked → manual data.go.kr search → download EUC-KR CSV → convert encoding → hand-parse long-format + dedup | `kvote nec results <pk>` |
| **Core historical results** | repeat the above for every election | `kvote nec corpus --normalize` |
| **Opinion polls** | click the board → download a PDF per post → read tables by hand *(no official API)* | `kvote nesdc sync` |
| **Turnout & winners** (OpenAPI) | sign up → apply & wait for approval → issue API key → find codes → call XML, paginate, parse | `kvote api login` *(once)* → `kvote nec winners <sgId>` |

## Features

> Output is `-f json|jsonl|table`. **Every command works without a key except those marked 🔑.**
> 🔑 = needs a data.go.kr API key (auto-issued via `kvote api`) · 🧪 = experimental (the keyless path comes first).

| Group | Capability | Command | Key |
|---|---|---|:--:|
| **⭐ Core** | Concurrent download + per-precinct normalization of core historical results | `nec corpus --normalize` | — |
| Results | Normalize results per precinct (aggregation, early/election-day split) | `nec results <pk>` | — |
| Results | Search public datasets (vote counts, etc.) | `nec datasets -q <query>` | — |
| Results | Auto-resolve the latest edition of an election type | `nec latest <keyword>` | — |
| Results | Download the original file (CSV/XLSX, auto-detected) | `nec pull <pk>` | — |
| Polls | Collect an entire range/condition as JSONL | `nesdc sync` | — |
| Polls | Cumulative master workbook → party support (2023-10-30~, 1,400+ records) | `nesdc bulk` | — |
| Polls | Single-record metadata + sample-composition crosstab | `nesdc show <nttId> --crosstab` | — |
| Polls | Download attached PDFs (tables/questionnaires) | `nesdc pull <nttId>` | — |
| Health | Live check of the killer paths (detects site-redesign breakage) | `doctor` | — |
| 🧪 OpenAPI | Turnout (by province / district, early vs. election-day) | `nec turnout <sgId> --sgtype N` | 🔑 |
| 🧪 OpenAPI | Winners (district · ballot no. · party · name · votes) | `nec winners <sgId> --sgtype N` | 🔑 |
| 🧪 OpenAPI | Election-code registry (every sgId / type since 1987) | `nec elections` | 🔑 |
| 🧪 Account | One-time browser login → keep the session alive | `api login` | — |
| 🧪 Account | My access applications (status · **expiry date**) | `api list` | 🔑 |
| 🧪 Account | Auto-submit an OpenAPI access application (purpose required, confirm) | `api apply <pk> --purpose` | 🔑 |

<details>
<summary><strong>Key options</strong></summary>

| Command | Key options |
|---|---|
| `nec corpus` | `--normalize` per-precinct JSONL on download · `-o` output dir · `--concurrency` |
| `nec results` | `--file` local parse · `--aggregate {town\|sgg\|sido\|national}` · `--by-votetype` · `--race`/`--leaf-only` (XLSX) |
| `nec datasets`/`pull` | `--source datagokr\|openportal` (open portal: data.nec.go.kr, uses dataId) |
| `nec turnout`/`winners` | `--sgtype` 1=president 2=parliament 3=governor 4=mayor 5=provincial 6=municipal · `--api-key` (default `KVOTE_DATAGOKR_KEY`) |
| `nesdc sync` | `-q`·`--field`·`--from`/`--to`·`--date-field`·`--gubun`·`--max-pages`·`--pull` |
| `api apply` | `--purpose` (required) · `--category research\|web\|app\|ref\|etc` · `--yes` |
| `api config` | `--auto-apply true` (submit without a confirmation prompt) |

</details>

### `nec corpus` — the killer feature

One line — `nec corpus --normalize` — concurrently downloads the core historical results (presidential 16th/17th/21st ·
parliamentary + proportional · local 5th–8th) and normalizes each to per-precinct JSONL on the spot. "Download + prep"
in a single command.

```bash
kvote nec corpus --normalize -o ./corpus
duckdb -c "SELECT * FROM read_json_auto('./corpus/*.jsonl') LIMIT 5"
```

### What normalization gives you (neutral parameters)

| Dimension | Detail |
|---|---|
| Vote type | election-day / in-district early / out-of-district early / absentee — classified per row |
| Multi-level aggregation | precinct → town → district → province → nationwide (metrics summed + turnout) |
| Derived values | turnout (votes/electorate) · valid votes (votes−invalid) · candidate share (votes/valid) — definitions stated |
| Raw preserved | electorate · votes · invalid · abstention · per-candidate votes — compute any identity yourself |
| Sample composition | poll gender/age/region × completed·weighted counts + weighting method |

### What you can verify

More agent-ready recipes live in `AGENTS.md`.

| Check | How |
|---|---|
| Early vs. election-day vote share | `nec results --aggregate sgg --by-votetype` |
| Count identity (votes = candidate sum + invalid) | `nec results` → compute from raw |
| Poll sample representativeness / weighting | `nesdc show --crosstab` |
| Poll distribution by agency × method | `nesdc sync` → aggregate the full set |
| Party-support time series | `nesdc bulk` |

## Install

**`go install` (recommended)**

```bash
go install github.com/JungHoonGhae/k-vote-cli/cmd/kvote@latest
# While the repo is private, git auth + GOPRIVATE are needed:
#   GOPRIVATE=github.com/JungHoonGhae/* go install github.com/JungHoonGhae/k-vote-cli/cmd/kvote@latest
```

**Homebrew (macOS/Linux)** — after the repo is public + tap is set up:

```bash
brew install JungHoonGhae/k-vote-cli/kvote
```

**From source**

```bash
git clone https://github.com/JungHoonGhae/k-vote-cli
cd k-vote-cli
make build        # -> bin/kvote
```

> Cross-platform binaries (macOS/Linux/Windows · arm64/amd64) are published to
> [GitHub Releases](https://github.com/JungHoonGhae/k-vote-cli/releases) by goreleaser on every `vX.Y.Z` tag.

## Quick Start

### For Agent

```text
Install k-vote-cli: go install github.com/JungHoonGhae/k-vote-cli/cmd/kvote@latest

Run `kvote doctor` first to confirm the core paths are alive.
Vote counts need no key:
  kvote nec corpus --normalize -o ./corpus   # core historical results → per-precinct JSONL
  kvote nec results <pk> -f jsonl             # normalize a single dataset
Take output as -f jsonl and query with jq/duckdb. k-vote-cli only provides data and
makes no normal/abnormal judgment — interpretation is the caller's.
OpenAPI (turnout/winners) needs a key, but turnout & winners are also derivable from results.
```

### For Human

```bash
# Core — historical results in one command
kvote nec corpus --normalize -o ./corpus
duckdb -c "SELECT * FROM read_json_auto('./corpus/*.jsonl') LIMIT 5"

# Single dataset: search → normalize
kvote nec datasets -q 개표결과 -f table
kvote nec results 15025527 -f jsonl > votes.jsonl     # per precinct, with candidate votes
kvote nec results 15025527 --aggregate sgg --by-votetype -f jsonl   # district × vote type

# Opinion polls — full collection
kvote nesdc sync --from 2026-01-01 > surveys.jsonl
kvote nesdc show 19366 --crosstab -f table            # single record + sample composition
kvote nesdc bulk -f jsonl > polls.jsonl               # cumulative party support

# data.go.kr OpenAPI — from key issuance to call (experimental)
kvote api login                                       # one-time browser login
kvote api apply 15000900 --purpose "election data analysis research"
export KVOTE_DATAGOKR_KEY=<service key>
kvote nec winners 20240410 --sgtype 2 -f jsonl        # 22nd parliamentary winners (254)
```

### Filters (`nesdc sync`)

| Flag | Meaning | Values |
|---|---|---|
| `-q`, `--query` | search term | free text |
| `--field` | field to match | `agency` `client` `method` `frame` `name` `sido` `regno` |
| `--from` / `--to` | date range | `YYYY-MM-DD` |
| `--date-field` | range basis | `registered` (default) `published` `surveyed` |
| `--gubun` | election segment | segment code (e.g. `VT044` 22nd presidential) |

> The portal silently ignores a date range without `--date-field` — k-vote-cli auto-applies `registered` when a range is given.

### Boards (`[board]` argument)

| name | content |
|---|---|
| `results` | poll results view (detailed metadata + PDF) — default |
| `data` | poll-result key data (weekly bulk) |
| `notices` · `library` · `actions` · `violations` | notices · library · actions · violations |

## Output formats · global flags

| `-f` | use |
|---|---|
| `json` | default. Read it, or shape with `jq` |
| `jsonl` | one record per line. Bulk/streaming (straight into jq·duckdb·pandas) |
| `table` | view in the terminal (CJK-width aligned) |

| Global flag | Default | Description |
|---|---|---|
| `-f, --format` | `json` | output format |
| `--delay` | `700ms` | minimum spacing between requests (rate limit) |
| `--base-url` | — | override the portal base URL (testing) |

## Data sources

| provider | site | content | key |
|---|---|---|:--:|
| `nec` | National Election Commission → data.go.kr | vote counts (files) + turnout·winners (OpenAPI) | files — / API 🔑 |
| `nesdc` | Election Survey Deliberation Commission (nesdc.go.kr) | poll results · sample composition | — |
| `api` | data.go.kr account | OpenAPI access application · key · expiry | 🔑 |

`nec` does **not** scrape the election-statistics site (info.nec.go.kr), which blocks all crawling via robots.txt.
Instead it searches and downloads the **vote-count files (CSV/XLSX) the NEC publishes on data.go.kr** — its official
distribution channel — without a key. `nesdc.go.kr` has no official API, so scraping is the only programmatic access.

## Development

```bash
make build    # build
make test     # tests (no network — uses testdata fixtures)
make fmt      # gofmt
go vet ./...
```

See [CLAUDE.md](CLAUDE.md) for internals and [AGENTS.md](AGENTS.md) for agent recipes.

## License

MIT
