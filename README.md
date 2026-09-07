<img width="1680" height="640" alt="image" src="https://github.com/user-attachments/assets/ade748ff-3f3b-4670-9bf9-d0c08e5f61a8" />

<div align="center">

# >_ ralph

<p><strong>AI agents, orchestrated. Development, accelerated.</strong></p>
<p>A multi-agent CLI harness for Claude Code & Codex projects.</p>

[![Release](https://img.shields.io/github/v/release/thomas0124/ralph?style=flat-square&color=blue)](https://github.com/thomas0124/ralph/releases)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/thomas0124/ralph/release.yml?style=flat-square&label=CI)](https://github.com/thomas0124/ralph/actions)
[![Homebrew](https://img.shields.io/badge/homebrew-thomas0124%2Ftap-orange?style=flat-square&logo=homebrew)](https://github.com/thomas0124/homebrew-tap)

</div>

---

## 🛠️ Quick Start

Initialize the AI development harness in your current project:

```sh
$ ralph init
✓ Initializing AI development harness
✓ Setting up agents and workflows
✓ Ready to build.
```

Start the orchestration terminal (`herdr`):

```sh
$ ralph status
```

Explore more commands:
- `ralph migrate`: Run workspace migrations.
- `ralph upgrade`: Upgrade agent templates and configurations.
- `ralph pack`: Manage language packs.

---

## 📦 Installation

<details>
<summary><strong>Homebrew（Recomended）</strong></summary>

```sh
brew install thomas0124/tap/ralph
```

</details>

<details>
<summary><strong>Go install</strong></summary>

Make sure Go is installed, then install the latest version:

```sh
go install github.com/thomas0124/ralph/cmd/ralph@latest
```

</details>

<details>
<summary><strong>Pre-compiled binaries</strong></summary>

- You can download the pre-compiled binaries from the [Releases](https://github.com/thomas0124/ralph/releases/) page.

</details>

---

## Command Reference

| Command | Description |
|---------|------|
| `ralph init` | Set up the harness in a project |
| `ralph upgrade` | Upgrade to the latest templates using a three-way merge |
| `ralph doctor` | Upgrade to the latest templates using a three-way merge |
| `ralph pack add <lang>` | Add a language pack |
| `ralph insights` | Aggregate and display pipeline execution data |
| `ralph org <verb>` | Run the autonomous multi-seat org runtime |
| `ralph status` | Show active seats |
| `ralph version` | Print the installed version |

---

## Operating Loop

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│                                                                                      │
│   /spec  ──▶  /plan  ──▶  /work                                                      │
│                               │                                                      │
│                               ▼                                                      │
│              /self-review ──▶ /verify ──▶ /test                                      │
│                                               │                                      │
│                                               ▼                                      │
│                                               /sync-docs ──▶ /cross-review ──▶ /pr   │
│                                                                （optional)　　　　　　  |
└──────────────────────────────────────────────────────────────────────────────────────┘
```

 After `/work`, the post-pipeline can be executed automatically by sub-agents.

| Skill | Purpose |
|--------|------|
| `/spec` | Clarify requirements interactively using a decision-tree workflow |
| `/plan` | Create a clean-base worktree and produce an implementation plan |
| `/work` | Implement a slice, commit it, and trigger the post-pipeline |
| `/self-review` | Check diff quality immediately after implementation |
| `/verify` | Check specification compliance and run static analysis |
| `/test` | Run behavioral tests |
| `/sync-docs` | Synchronize documentation |
| `/cross-review` | Request a cross-model second opinion (optional) |
| `/pr` | Create a pull request and clean up the worktree |

---

## Language Packs

```sh
ralph pack add typescript   # tsc + eslint
ralph pack add python       # mypy + ruff + pytest
ralph pack add golang       # go vet + staticcheck + golangci-lint
ralph pack add rust         # cargo check + clippy + fmt
ralph pack add dart         # dart analyze + flutter/pure-dart test
ralph pack add terraform    # terraform validate + tflint (tofu preferred)
```

Each pack expands to:

```
packs/languages/<lang>/
├── verify.sh          ← POSIX sh verification script
├── rule.md            ← Language-specific rules
└── README.md
.claude/rules/ralph/<lang>.md   ← Rules loaded by Claude Code
```

`verify.sh` can be controlled with:

| Variable | Description |
|------|------|
| `HARNESS_VERIFY_MODE` | `static` / `test` / `all`（default: `all`） |
| `RALPH_VERIFY_PROJECT_ROOTS` | Space-separated project roots to verify in a monorepo |

---

## `ralph doctor` — Health Checks

```sh
ralph doctor
# Also probe model connectivity for the org runtime:
ralph doctor --probe-models
```

| Check | Description |
|-------------|------|
| Claude CLI | Required/warn-only behavior controlled by `require_claude_cli` |
| Codex CLI | Required when `require_codex_cli = true` |
| OpenCode CLI | Required when `require_opencode_cli = true` (for org runtime opencode driver users) |
| Go | Required when `require_go = true` |
| settings.json | Validates hook configuration |
| Hook scripts | Checks executable bits |
| Manifest | Validates version and hash consistency |
| Language packs | Checks `verify.sh` exists and is executable |
| herdr | Terminal pane manager required by the org runtime |
| agmsg | Seat-to-seat messaging scripts |
| org envelope | Connectivity checks for herdr + agmsg |
| model probes | With `--probe-models` , checks connectivity for each model driver |

Configure these requirements in `ralph.toml` :

```toml
[doctor]
require_claude_cli   = true   # false: warn only
require_codex_cli    = false  # true: require Codex CLI
require_opencode_cli = false  # true: require OpenCode CLI
require_go           = false  # true: require Go
```

---

## `ralph insights` — Pipeline Analytics

```sh
ralph insights              # Display aggregated events
ralph insights --json       # Output JSON
ralph insights backfill     # Generate historical data from docs/reports/
```

`ralph insights` aggregates the JSONL events stored under `docs/insights/events/` and reports:

- Phase-level metrics (`phase` / `events` / `verdicts` / `findings` / `triage`)
- Escalation comparisons for slugs reaching `cycle >= 2`
- Routing statistics, including honored rate per phase

---

## Org Runtime — Autonomous Multi-Seat Execution

The `org` runtime combines **herdr** (terminal pane management) and **agmsg** (typed messaging) to run multiple AI seats in parallel. Claude Code, Codex, and OpenCode can be mixed in the same runtime. OpenCode is opt-in (see `driver_pool` below).

### Prerequisites

| Tool | Installation |
|--------|-------------|
| [herdr](https://herdr.dev) | `brew install herdr` |
| [agmsg](https://github.com/fujibee/agmsg) | `git clone https://github.com/fujibee/agmsg ~/.agents/skills/agmsg` |

```sh
ralph doctor
```

### Topology

```
lead (Claude Code)
  ├── seat: implementer  (driver: claude / codex)
  ├── seat: reviewer     (driver: claude)
  └── seat: tester       (driver: codex / opencode)
```

The lead seat dispatches tasks to individual seats and coordinates their RESULT messages. Seats are launched as terminal panes by `herdr` ; seat-to-seat communication uses agmsg message types such as `TASK` , `RESULT` , `BLOCKED` , `REVIEW` , and `QUESTION` .

### `ralph org`  Commands

```sh
# Spawn seats (--org-id, --id, --role, --driver, --model, --cwd are all required)
# Default permission mode is "autonomous", which requires --scope (or --allow-unscoped to bypass)
ralph org spawn --org-id demo --id reviewer --role reviewer --driver claude --model sonnet --cwd . --scope "review code changes"
ralph org spawn --org-id demo --id verifier --role verifier --driver codex  --model gpt-5.5 --cwd . --scope "verify test results"

# opencode: add it to driver_pool / model_pool in ralph.toml first; supports autonomous or guarded only
# ralph org spawn --org-id demo --id tester --role tester --driver opencode --model <model> --cwd . --scope "run tests"

# Send a task (--to = target seat id, --text = message body)
ralph org send --org-id demo --to reviewer --text "TYPE: TASK
TASK_ID: t-1
SEAT: reviewer

<task body>"

ralph org wait  --org-id demo --seat reviewer   # Wait for completion
ralph org read  --org-id demo --seat reviewer   # Read the latest message
ralph org stop  --org-id demo --seat reviewer   # Stop a seat
ralph org disband --org-id demo                 # Stop all seats
ralph status                                    # List active seats
```

### `ralph.toml`  Org Configuration

```toml
[org]
driver_pool      = ["claude", "codex"]
model_pool       = [
  { driver = "claude", model = "opus" },    # Decision-making and review
  { driver = "claude", model = "sonnet" },  # Implementation
  { driver = "codex",  model = "gpt-5.5" }, # Verification
]
max_seats        = 5
deadman_minutes  = 10
agmsg_home       = "~/.agents/skills/agmsg"

[org.permissions]
default          = "default"
codex_verified   = false   # true: grant broader permissions to Codex

[org.budget]
seat_wall_clock_minutes  = 30
total_wall_clock_minutes = 120
max_fix_rounds           = 3
```

#### Using OpenCode as a seat (opt-in)

Add `"opencode"` to `driver_pool` and a `provider/model` entry to `model_pool`.
The model name is passed verbatim to `opencode run --model`.

```toml
[org]
driver_pool = ["claude", "codex", "opencode"]
model_pool = [
  # ...,
  { driver = "opencode", model = "anthropic/claude-sonnet-4-5" },
]

[doctor]
require_opencode_cli = false   # true: require OpenCode CLI
```

Permission mode mapping: autonomous → `--auto` , guarded → no flag. `edits`
has no equivalent in OpenCode's run CLI and fails closed (error).

Initial prompts are driver-dependent: Claude/Codex seats receive them as a
trailing positional argument at launch (pre-filled, never auto-submitted).
OpenCode seats launch bare and the prompt line is typed into the input box
once herdr reports the TUI ready — OpenCode's `--prompt` flag auto-submits,
which would keep the seat busy past herdr's readiness detection.

### Claude Code + Codex Bridging

The `/cross-review` skill enables an asynchronous workflow in which one model implements and the other reviews:

```
Claude implements → Codex reviews
Codex implements  → Claude reviews
```

`/cross-review` also works independently of the org runtime.

---

## Model Routing

```
┌──────────────────────────────────────────────────────┐
│  orchestrator  │  User-selected model (session)      │
│  reviewer      │  opus — decisions and evaluation    │
│  verifier      │  sonnet — verification, tests, impl │
│  implementer   │  sonnet                             │
│  bulk lookups  │  haiku  — grep / file inspection    │
└──────────────────────────────────────────────────────┘
```

> Important: Always declare model: explicitly in agent frontmatter. Omitting it causes inheritance and can unintentionally launch more expensive models multiple times.

## ライセンス

[MIT](LICENSE) © thomas0124
