<p align="right"><a href="README.md">한국어</a> · <strong>English</strong></p>

<div align="center">
  <h1>k-vote-cli</h1>
  <p><strong>Election results and this week's polls — one command, for anyone.</strong></p>
  <p>Korean public election data with no special access, no sign-up. Humans and AI agents use the same command (<code>kvote</code>) and get analysis-ready output.</p>
  <p><sub>Historical ballot-count results <strong>normalized per polling station</strong>, <strong>bulk collection of this week's polls</strong>, party-support time series, and more. <a href="#features">Full feature table ↓</a></sub></p>
</div>

<p align="center">
  <a href="#quick-start"><strong>Quick Start</strong></a> ·
  <a href="#features"><strong>Features</strong></a> ·
  <a href="#install"><strong>Install</strong></a> ·
  <a href="#what-you-can-verify"><strong>Verification examples</strong></a> ·
  <a href="CLAUDE.md"><strong>Internals</strong></a>
</p>

<p align="center">
  <a href="docs/k-vote-cli-promo.mp4"><img src="docs/k-vote-cli-promo.gif" alt="k-vote-cli promo: election results and this week's polls in one command" width="760" /></a>
</p>
<p align="center"><sub><a href="docs/k-vote-cli-promo.mp4">HD video</a> · <a href="docs/k-vote-cli.gif">terminal demo</a></sub></p>

> [!WARNING]
> **Some features** (poll collection and the dataset search/download paths) depend on the HTML
> structure of public portals, so a site redesign can break those paths — `kvote doctor` checks
> them instantly. File parsing, the local DB, and MCP work regardless of portal changes.
> Everything collected is **public data with a legal disclosure mandate** (materials registered and
> published under Korea's election-poll regulations), fetched politely with a default rate limit.
> Use at your own risk.

## Why this exists

Distrust in how elections are administered runs higher in Korea than it has in a long time.
What stays out of reach stays unknown — and the unknown is where fear and distrust take root.
And in an era when anyone can analyze anything with an AI at their side, election data alone
still sits behind blocked portals, legacy formats, and API keys that must be applied for one
dataset at a time — hard to reach for people and for AI alike.

This tool cannot settle the question of distrust. But as one developer, I wanted to fix this
one problem: even if it is nowhere near enough, the data that already exists should at least
be easier to reach. And when more people can open things up and verify them for themselves,
perhaps we can build, together, a system that earns a little more trust. That vague hunch is
where this project began.

You want to check an election result yourself, or see how this week's polls came out.
The data is "public" — but where do you download it, which of the dozens of similar datasets is the
right one, and why is the file garbled when you finally get it? info.nec.go.kr blocks automated
access, poll data has no API at all (a PDF per bulletin post), and the OpenAPI route requires
sign-up, application forms, and key issuance. It is a wall even for developers, let alone everyone else.

`k-vote-cli` removes that wall. You don't need to know where the data lives or which file to pick.
One command finds the right source, downloads it, and hands it to you in an analysis-ready shape —
whether a person typed the command or an AI agent called it.

<details>
<summary>What exactly was blocked</summary>

- The election statistics system (info.nec.go.kr) fully disallows automated access (robots.txt)
- Files come in legacy encodings (EUC-KR), inconsistent spreadsheet layouts, duplicated polling-station rows
- The data.go.kr OpenAPI requires account sign-up, usage applications, and key issuance

k-vote-cli absorbs all of it.
</details>

> **Neutral by design.** This project advocates nothing. Whatever question brings you here and
> whatever conclusion you leave with, everyone starts from the same data, fetched the same way.
> Raw values are preserved verbatim; only clearly defined standard derivations (turnout, vote
> share, sums) are added. The judgment belongs to whoever receives the data.

## What makes it different

- **All major historical results in one command** — presidential, general, and local elections, normalized per polling station (`nec corpus`)
- **Every poll, this week included** — all registered surveys plus party-support time series (`nesdc sync` · `bulk`)
- **AI handles the API-key paperwork** — data.go.kr sign-in, applications, approval status (`api login` · `apply`)
- **Raw data, reproducible forever** — same command, same result, for anyone

## Before → After

| Data | Before | k-vote-cli |
|---|---|---|
| **Ballot counts** (per polling station) | info.nec.go.kr blocked → manual search on data.go.kr → CSV in EUC-KR → re-encode → hand-parse long format and de-duplicate | `kvote nec results <pk>` |
| **All major historical results** | Repeat the above for every election | `kvote nec corpus --normalize` |
| **Opinion polls** | Click through a bulletin board, download a PDF per post, read tables by hand *(no official API)* | `kvote nesdc sync` |
| **Turnout / winners** (OpenAPI) | Sign up → apply and wait for approval → get a key → find codes → call XML, paginate, parse | `kvote api login` *(once)* → `kvote nec winners <sgId>` |

## Features

> Output is `-f json|jsonl|table`. Commands marked **O** in the **No API key** column work
> immediately, with no sign-up or key issuance. **X** = needs a data.go.kr key
> (`kvote api` automates issuance). The OpenAPI and Account groups are experimental.

| Group | Feature | Command | No API key |
|---|---|---|:--:|
| **Core** | Download all major historical results at once + normalize per polling station | `nec corpus --normalize` | **O** |
| Results | Normalize a result file per polling station (aggregation, election-day/early split) | `nec results <pk>` | **O** |
| Results | Search public file datasets | `nec datasets -q <keyword>` | **O** |
| Results | Resolve the latest edition of an election type | `nec latest <keyword>` | **O** |
| Results | Download the raw file (CSV/XLSX autodetected) | `nec pull <pk>` | **O** |
| Turnout | Normalize turnout-analysis ZIPs by gender, age group, and region | `nec turnout-analysis <pk>` | **O** |
| Polls | Bulk-collect every survey in a date range as JSONL | `nesdc sync` | **O** |
| Polls | Cumulative master spreadsheet → party support (1,400+ polls since 2023-10-30) | `nesdc bulk` | **O** |
| Polls | Per-survey metadata + sample-composition cross-tabs | `nesdc show <nttId> --crosstab` | **O** |
| Polls | Download survey attachments (tables, questionnaires) | `nesdc pull <nttId>` | **O** |
| Polls | Fetch exactly the result-table PDF (single or `--sync` batch) | `nesdc tabulation <nttId>` | **O** |
| Local DB | Load results/polls/turnout into SQLite + read-only SQL | `db ingest …` · `db query` | **O** |
| AI agents | MCP server (discover, ingest, and SQL-query via tools) | `mcp` | **O** |
| Health | Live check of the core paths (detects portal redesigns) | `doctor` | **O** |
| OpenAPI | Turnout by province/district, election-day/early split | `nec turnout <sgId> --sgtype N` | X |
| OpenAPI | Winners (district, party, name, votes) | `nec winners <sgId> --sgtype N` | X |
| OpenAPI | Election-code registry (every sgId since 1987) | `nec elections` | X |
| Account | One browser sign-in, session persisted | `api login` | **O** |
| Account | Your API applications (status, **expiry dates**) | `api list` | X |
| Account | Auto-submit an OpenAPI usage application | `api apply <pk> --purpose` | X |

<details>
<summary><strong>Key options</strong></summary>

| Command | Options |
|---|---|
| `nec corpus` | `--normalize` per-station JSONL on download · `-o` output dir · `--concurrency` |
| `nec results` | `--file` parse a local file · `--aggregate {town\|sgg\|sido\|national}` · `--by-votetype` · `--race`/`--leaf-only` (XLSX) |
| `nec datasets`/`pull` | `--source datagokr\|openportal` (open portal: data.nec.go.kr, uses dataId) |
| `nec turnout`/`winners` | `--sgtype` 1=president 2=assembly 3=governor 4=mayor 5=provincial council 6=municipal council · `--api-key` (default `KVOTE_DATAGOKR_KEY`) |
| `nesdc sync` | `-q` · `--field` · `--from`/`--to` · `--date-field` · `--gubun` · `--max-pages` · `--pull` |
| `api apply` | `--purpose` (required) · `--category research\|web\|app\|ref\|etc` · `--yes` |
| `api config` | `--auto-apply true` (submit without confirmation) |

</details>

### `nec corpus` — the flagship

One line — `nec corpus --normalize` — downloads the major historical ballot-count results
(presidential #16/#17/#21, general + proportional, local #5–#8) in parallel and normalizes them
into per-polling-station JSONL as they arrive. Ready to look at the moment it lands.

```bash
kvote nec corpus --normalize -o ./corpus
duckdb -c "SELECT * FROM read_json_auto('./corpus/*.jsonl') LIMIT 5"
```

### What normalization gives you

| Dimension | Detail |
|---|---|
| Vote type | Every row labeled election-day / early (in-district) / early (out-of-district) / mail-ship |
| Multi-level aggregation | Polling station → town → district → province → national (sums + turnout) |
| Derived values | Turnout (votes/electorate) · valid votes (votes−invalid) · candidate share (votes/valid) — all with explicit definitions |
| Raw data preserved | Electorate, votes, invalid, abstentions, per-candidate counts — every identity can be recomputed |
| Sample composition | Poll samples by gender/age/region × completed/weighted counts + weighting method |

## What you can verify

More agent-oriented recipes live in `AGENTS.md`.

| Question | How |
|---|---|
| Early vs election-day vote share | `nec results --aggregate sgg --by-votetype` |
| Count identity (votes = candidates + invalid) | `nec results` → recompute from raw fields |
| Poll sample representativeness / weighting | `nesdc show --crosstab` |
| Poll distribution by agency × method | `nesdc sync` → aggregate |
| Party-support time series | `nesdc bulk` |
| Turnout by gender and age group | `nec turnout-analysis <pk>` — same axes (gender/age) as poll sample cross-tabs, so polls, turnout, and results can be compared side by side |

## Which elections are covered

kvote hardcodes no election — **anything the commission publishes on the portal becomes available
automatically** (check the live list anytime with `nec datasets` / `nec latest`). What is on the
portal today:

| Election | Available editions | Format | Notes |
|---|---|---|---|
| Presidential | 16th · 17th · 21st | CSV | No file data on the portal for the 18th–20th |
| National Assembly (general) | 22nd (districts + proportional) | CSV | |
| Nationwide local elections | 5th – 8th | XLSX | Seven simultaneous races (governors, mayors, superintendents, councils) in one file |
| By-elections | Combined file | XLSX | |
| Turnout analysis (gender/age) | Editions published as XLSX (20th–21st presidential, 21st–22nd general, 8th local, …) | ZIP | PDF-only older editions: raw download via `nec pull` |
| Opinion polls | Entire board + cumulative party support (since 2023-10-30) | HTML/XLSX/PDF | |

**Easy to confuse**

- **Numbering differs by election type.** Presidential and general elections count in "제N**대**"
  (21st presidential = 2025, 22nd general = 2024); local elections count in "제N**회**"
  (8th = 2022, 9th = 2026). A "19th" exists for the presidential (2017) and general (2012)
  elections — but not for local elections.
- **A just-finished election is not there yet.** The commission takes time to publish the file
  data (e.g. the 9th local elections of June 2026 are not posted yet). The moment it lands, the
  same commands pick it up — `kvote nec latest "지방선거 개표결과"` resolves the newest edition
  automatically.
- **One local-election file holds several races.** Governors, mayors, superintendents, and
  councils are separate sheets; select one with `nec results --race 시·도지사`.
- **"Turnout" means two different datasets.** Turnout derived from ballot counts
  (votes/electorate) is one thing; the turnout *analysis* (`nec turnout-analysis`) tells you
  **who voted** (by gender and age group) — an axis that ballot counts do not contain.

## Install

**`go install` (recommended)**

```bash
go install github.com/JungHoonGhae/k-vote-cli/cmd/kvote@latest
```

**Homebrew (macOS/Linux)**

```bash
brew install JungHoonGhae/k-vote-cli/kvote
```

**Build from source**

```bash
git clone https://github.com/JungHoonGhae/k-vote-cli
cd k-vote-cli
make build        # -> bin/kvote
```

> Cross-platform binaries (macOS/Linux/Windows · arm64/amd64) are published automatically to
> [GitHub Releases](https://github.com/JungHoonGhae/k-vote-cli/releases) by goreleaser on every `vX.Y.Z` tag.

## Quick Start

### For agents

```text
Install: go install github.com/JungHoonGhae/k-vote-cli/cmd/kvote@latest

Run `kvote doctor` first to confirm the core paths are alive.
Ballot-count results need no API key:
  kvote nec corpus --normalize -o ./corpus   # major historical results → per-station JSONL
  kvote nec results <pk> -f jsonl             # normalize a single dataset
Prefer -f jsonl and query with jq/duckdb.
The OpenAPI commands (turnout/winners) need a key, but turnout and winners
can also be derived from `nec results`.
```

### For humans

```bash
# The one-liner: all major historical results
kvote nec corpus --normalize -o ./corpus
duckdb -c "SELECT * FROM read_json_auto('./corpus/*.jsonl') LIMIT 5"

# A single dataset: search → normalize
kvote nec datasets -q 개표결과 -f table
kvote nec results 15025527 -f jsonl > votes.jsonl     # per polling station, with candidate votes
kvote nec results 15025527 --aggregate sgg --by-votetype -f jsonl   # district × vote type

# Polls: bulk collection
kvote nesdc sync --from 2026-01-01 > surveys.jsonl
kvote nesdc show 19366 --crosstab -f table            # one survey + sample composition
kvote nesdc bulk -f jsonl > polls.jsonl               # cumulative party support

# data.go.kr OpenAPI: from key issuance to calls (experimental)
kvote api login                                       # one browser sign-in
kvote api apply 15000900 --purpose "election data research"
export KVOTE_DATAGOKR_KEY=<your key>
kvote nec winners 20240410 --sgtype 2 -f jsonl        # all 254 winners of the 22nd general election
```

## MCP server (for AI agents)

`kvote mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io) server over stdio,
so agents reach the same data through tool calls instead of shell commands.

### Why load a local DB at all?

The local DB is **optional, not required**. For one-off analysis, JSONL from
`nec results`/`nesdc sync` piped into `jq`/`duckdb` is faster. Loading pays off when you need:

- **Cross-dataset queries** — join and aggregate multiple elections, results, and polls in SQL instead of juggling JSONL files.
- **Repeated reuse** — download and normalize hundreds of thousands of polling-station rows once; every later query is instant.
- **Standard derivations in one SELECT** — turnout, vote share, valid votes, and multi-level rollups are pre-defined as views (`v_results_derived`, `v_agg_*`) whose SQL is the definition (neutral). See the `kvote://schema` resource.
- **Agent ergonomics** — with the MCP `query` tool, an agent turns natural language into SQL and slices the data itself. No shell pipelines, no file management.

Raw data is stored verbatim; the DB adds convenience, **never judgment.**

Every tool works without an API key.

| tool / resource | What it does |
|---|---|
| `search_datasets` | Keyword search over data.go.kr file datasets |
| `list_elections` | Resolve the latest edition of an election type |
| `ingest_results` | Download ballot-count results into local SQLite (idempotent) |
| `ingest_polls` | Download the cumulative poll spreadsheet and load it (idempotent) |
| `ingest_turnout` | Download turnout analysis (gender/age) and load it (idempotent) |
| `query` | Read-only SQL against the local DB |
| `kvote://schema` | Table/view schema + derivation definitions (read before `query`) |

Raw values are stored as-is; derived values exist only as view SQL. `query` uses a read-only
connection, so the engine itself rejects writes — judgment stays with the agent or the person.

Register with Claude Code:

```bash
claude mcp add kvote -- kvote mcp
```

Or in a config file:

```json
{
  "mcpServers": {
    "kvote": { "command": "kvote", "args": ["mcp"] }
  }
}
```

### `kvote db` (the same SQLite from the CLI)

The same local DB works without MCP:

```bash
kvote db ingest results <publicDataPk>   # ballot-count CSV → load (idempotent)
kvote db ingest polls                     # cumulative poll spreadsheet → load (idempotent)
kvote db ingest turnout <publicDataPk>    # turnout analysis (gender/age) → load (idempotent)
kvote db query "SELECT * FROM v_agg_sgg LIMIT 5" -f table
```

The DB lives in the OS config directory (`…/kvote/kvote.db`) by default; override with the global `--db` flag.

### Filters (`nesdc sync`)

| Flag | Meaning | Values |
|---|---|---|
| `-q`, `--query` | Search keyword | free text |
| `--field` | Field to search | `agency` `client` `method` `frame` `name` `sido` `regno` |
| `--from` / `--to` | Date range | `YYYY-MM-DD` |
| `--date-field` | Which date the range applies to | `registered` (default) `published` `surveyed` |
| `--gubun` | Election category code | e.g. `VT044` for the 22nd presidential election |

> The portal silently ignores date ranges without a date field; when you pass a range,
> `registered` is applied automatically.

### Boards (the `[board]` argument)

| name | Content |
|---|---|
| `results` | Published survey results (metadata + PDFs) — default |
| `data` | Weekly bulk data posts (cumulative spreadsheet) |
| `notices` · `library` · `actions` · `violations` | Notices, library, sanctions, violation cases |

## Output formats · global flags

| `-f` | Use |
|---|---|
| `json` | Default. Read directly or pipe to `jq` |
| `jsonl` | One record per line. Bulk collection/streaming (jq · duckdb · pandas) |
| `table` | Terminal viewing (CJK-aware column alignment) |

| Global flag | Default | Description |
|---|---|---|
| `-f, --format` | `json` | Output format |
| `--delay` | `700ms` | Minimum interval between requests (rate limit) |
| `--base-url` | - | Override the portal base URL (for tests) |
| `--db` | OS config dir | Local SQLite path (used by `kvote mcp` / `kvote db`) |

## Data sources

| provider | Site | Content | No API key |
|---|---|---|:--:|
| `nec` | National Election Commission → data.go.kr | Ballot-count files + turnout/winners (OpenAPI) | files **O** / OpenAPI X |
| `nesdc` | National Election Survey Deliberation Commission (nesdc.go.kr) | Poll results and sample composition | **O** |
| `api` | data.go.kr account | OpenAPI applications, keys, expiry | X |

`nec` does **not scrape** the election statistics system (info.nec.go.kr), whose robots.txt
disallows all crawling. It uses the commission's official distribution channel instead — the
CSV/XLSX result files on **data.go.kr**, downloadable without an API key. `nesdc.go.kr` has no
official API, so scraping is the only programmatic access.

## Development

```bash
make build    # build
make test     # tests (no network; testdata fixtures)
make fmt      # gofmt
go vet ./...
```

Internals are documented in [CLAUDE.md](CLAUDE.md); agent recipes in [AGENTS.md](AGENTS.md).

## Contributing

Issues and PRs welcome. Please read the **two non-negotiable principles** (neutrality · no API key)
in [CONTRIBUTING.md](CONTRIBUTING.md) first.

- Changelog: [CHANGELOG.md](CHANGELOG.md)
- Code of conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- Security reports: [SECURITY.md](SECURITY.md) (please do not open public issues)

## License

[MIT](LICENSE)
