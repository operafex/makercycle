[Português (Brasil)](README.pt-BR.md)

# MakerCycle

Autonomous multi-agent CLI orchestrator for closed-loop software engineering (SCAN → PLAN → CHALLENGE → MAKE → VERIFY → GATE). Spec-driven, strict TDD and automated security auditing. Written in Go with **zero external dependencies** (standard library only).

## One Cycle, Six Stages

**MakerCycle (`makercycle`)** is the complete cycle. Each internal phase is a **Stage**:

| Stage | Name | Agent | Client | Default Model | What it does |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 0 | `SCAN` | `SCOUT` | `claude` | `opus` | Maps an existing project and consolidates its documentation into `<docs>/PROJECT_CONTEXT.md`. |
| 1 | `PLAN` | `PLANNER` | `claude` | `opus` | Incremental planning (`obra/superpowers`) and automated tests in `RED` state. |
| 2 | `CHALLENGE` | `CRITIC` | `agy` | `Gemini 3.8 Flash (Medium)` | Cold critical review and global invariants (`google/elephant-goldfish-model`). |
| 3 | `MAKE` | `MAKER` | `agy` | `Gemini 3.8 Flash (High)` | Implementation in a sanitized (*stateless*) context. |
| 4 | `VERIFY: CHECK` / `VERIFY: HARDEN` | `AUDITOR` | `agy` *(4B)* | `Gemini 3.8 Flash (Medium)` *(4B)* | Deterministic stack suite (4A) + AppSec audit of the `git diff` (4B, `cloudflare/security-audit-skill`). |
| 5 | `GATE` | `GATEKEEPER` | `agy` | `Gemini 3.8 Flash (High)` | Semantic judgment, token accounting and *Circuit Breaker* (`max-attempts`). |

> **Note:** In Stage 4 (`VERIFY`), phase 4A (`CHECK`) runs deterministic local stack tests and linters (no LLM), while phase 4B (`HARDEN`) executes the AppSec audit with the `AUDITOR` agent using the indicated client and model.

Stages 3 → 4 → 5 run as a closed loop until the GATEKEEPER approves, asks for a test amendment, escalates to the planner, or the attempt limit trips.

## Requirements

- **Go 1.22+** *(only to build from source or run with `go run .`; not needed when using the installers)*
- **Git** (used for repository state and `git diff` security audits)
- **At least one AI agent CLI** installed and configured (e.g., `claude`, `gemini`, `codex`, `opencode` — freely configurable per stage in `.makercycle/config/models.env`)
- **Node.js / `npx`** *(optional)*: required only when running `-init` to automatically install external skills configured in `models.env

## Installation

**With Go (recommended)** *(requires Go 1.22+)*

```bash
go install github.com/operafex/makercycle@latest
```

The binary is placed in `$(go env GOPATH)/bin` (usually `~/go/bin`). Add that folder to your `PATH` (zsh shown; on bash use `~/.bashrc`), check the binary and initialize your project:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
which makercycle
makercycle -init
```


**Without Go** — pre-built binaries are published on every release for `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`, `windows/amd64` and `windows/arm64`. Use the one-line installer for your system:

*macOS / Linux*

```bash
curl -fsSL https://raw.githubusercontent.com/operafex/makercycle/main/install.sh | sh
```

*Windows (PowerShell)*

```powershell
irm https://raw.githubusercontent.com/operafex/makercycle/main/install.ps1 | iex
```

The installers download the binary for your OS/architecture from GitHub Releases, **verify its SHA-256 against `checksums.txt`** (aborting on mismatch), install it and add the folder to your `PATH`.

| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `MAKERCYCLE_VERSION` | `latest` | Pin a release, e.g. `MAKERCYCLE_VERSION=v0.2.0` (format `vX.Y.Z[-pre]`) |
| `MAKERCYCLE_INSTALL_DIR` | `~/.local/bin` (macOS/Linux) · `%LOCALAPPDATA%\Programs\makercycle` (Windows) | Where the binary is installed |

Verify the installation:

```bash
makercycle -version
```

## Quick Start

### 1. Initialize in one command — `makercycle -init`

```bash
makercycle -init                # provision the project (safe to re-run)
makercycle -init -force         # also restore the agent contracts to the embedded templates
makercycle -init -force-skills  # also reinstall/update every skill
```

The binary is self-contained: the agent contracts (`.makercycle/agents/*.md`) and the `models.env` template are embedded in it. `makercycle -init`:

1. creates `.makercycle/{agents,config,state,telemetry}/` and extracts the six agent contracts to `.makercycle/agents/`. Existing files are **kept**; `-force` overwrites only `.makercycle/agents/*.md`;
2. creates `<docs>/demands/`, `<docs>/logs/` and `<docs>/references/` (each with a `.gitkeep`) and writes its own log to `<docs>/logs/`;
3. provisions `.makercycle/config/models.env`: an existing file is **never** overwritten (not even with `-force`); otherwise it is copied from `~/.makercycle/config/models.env` when that file exists (global inheritance), or written from the embedded template;
4. appends `.makercycle/state/`, `.makercycle/telemetry/` and `.makercycle/config/project.env` to `.gitignore` (idempotent; equivalent entries such as `/.makercycle/state` are recognized);
5. checks that the configured binaries (and `git`) are on your `$PATH` (`[OK]` / `[AUSENTE]`). `npx` is only required when skills are declared;
6. installs the skills declared in `MC_*_SKILL` (all of them with `-force-skills`). If none is declared, skill installation is skipped. A skill counts as present if a folder with its name exists in `skills/`, `.agents/skills/`, `~/.claude/skills/` or `~/.antigravity/skills/`, or if `skills-lock.json` lists it as a source;
7. detects and confirms the project stack (saved to `.makercycle/config/project.env`);
8. if the project already has source files, runs the **SCAN** (Stage 0, SCOUT) to generate `<docs>/PROJECT_CONTEXT.md`. Without valid API keys the SCAN is skipped with a warning (finish later with `-setup`).

It exits with code `1` if a binary is missing, a skill failed to install or the SCAN could not run.

#### Global `models.env` (`-save-global`)

Tune `.makercycle/config/models.env` once, then reuse it in every new project:

```bash
makercycle -save-global   # copies .makercycle/config/models.env to ~/.makercycle/config/models.env
```

The file is written atomically with mode `0600` (folder `0700`) and overwrites any previous global copy. New projects initialized with `makercycle -init` inherit `~/.makercycle/config/models.env` automatically. Never put API keys in this file — they belong in `.makercycle/config/project.env` (ignored by Git).

### 2. Onboard an existing project — `-setup` (Stage 0)

```bash
makercycle -setup
```

Creates `<docs>/PROJECT_CONTEXT.md`, `<docs>/demands/INDEX.md`, `<docs>/demands/`, `<docs>/logs/` and `<docs>/references/`.

### Auto-Detected & Custom Stacks

When running `-setup`, `makercycle` deterministically inspects markers in the project root:
- `go.mod` → Go (`go test -v -race ./...`, `go vet ./...`, and `golangci-lint run` if `.golangci.yml` or `.golangci.yaml` exists);
- `Gemfile` with `gem 'rails'` → Ruby on Rails (`bundle exec rspec` if `spec/` directory exists, otherwise `bin/rails test`, plus `bundle exec rubocop`);
- `package.json` → Node.js or TypeScript + Node.js (if `tsconfig.json` exists or `typescript` is in dependencies/devDependencies; `npm run lint` if `lint` is defined in `scripts`).

During `-setup`, the detected stack is shown and you are asked to confirm it before anything is written:

```text
[STACK] Detectada: Go (testes: go test -v -race ./...)
Confirmar esta stack como padrão do projeto? [S/n] (ou digite 'p' para personalizar): 
```

Enter, `S` or `s` confirms the detected stack; `p` (or `n`) opens the stack menu so you can pick a different stack; any other answer aborts the setup without writing. When no stack is detected, no question is asked. The `SCOUT` agent then confirms or refines the stack, and the configuration is saved to `.makercycle/config/project.env`:

```env
MC_STACK_NAME="Go"
MC_TEST_CMD="go test -v -race ./..."
MC_TYPE_CMD="go vet ./..."
MC_LINT_CMD=""
```

When you start a demand with `-task`, `-stage` or `-demand`, `makercycle` uses the stack saved in `.makercycle/config/project.env` silently — no stack confirmation is required on each run:

```text
[INFO] Stack do projeto: Go (via project.env)
```

To change the stack, pass `-select-stack`: it opens the menu even when `project.env` is valid and saves your choice. The menu also opens automatically when `project.env` is missing or incomplete.

```bash
go run . -task "Implement JWT authentication" -select-stack
```

The `-select-stack` menu looks like this:

```text
Qual stack deve ser utilizada nesta execução?
  [1] Projeto Atual (Auto-detectado): Go
  [2] Ruby on Rails
  [3] Go
  [4] TypeScript + Node.js
  [5] Personalizada
Escolha uma opção (1-5) [Enter = 1]: 
```

Pressing Enter defaults to option `1`. If `.makercycle/config/project.env` is missing or incomplete, a warning is displayed and the manual menu (options 1 to 4) is presented as fallback, and the chosen stack is saved. You can also edit `.makercycle/config/project.env` manually to override commands or configure custom stacks (such as Python, Java, or Makefile).

### 3. Run a new demand — `-task` (Stages 1 to 5)

```bash
go run . -task "Implement JWT authentication with refresh tokens"
```

Enrich the demand with material from `<docs>/references/` using `-ref` (repeatable). Each file is validated up front and handed to the PLANNER as `@path`, so it is read before planning:

```bash
go run . -task "Implement JWT authentication with refresh tokens" \
  -ref auth-spec.md -ref docs/references/openapi.yaml
```

`-ref` accepts either a path (relative to the current directory or absolute) or just a file name inside `<docs>/references/` (follows `-docs-dir` / `MC_DOCS_DIR`). A missing file aborts before any agent runs. `-ref` only affects Stage 1, so it is ignored (with a warning) when you start with `-stage 2` or later.

Start from a later Stage with `-stage`:

```bash
go run . -stage 2    # CHALLENGE onwards (reuses existing .makercycle/state/PLAN.md / tests)
go run . -stage 3    # MAKE onwards
```

### Autonomous specification loop

Stages 1 (PLANNER) and 2 (CRITIC) run as a closed loop. When the CRITIC writes `SPEC_STATUS: REVISION_REQUIRED`, Stage 1 is re-run automatically: the PLANNER receives `@.makercycle/state/SPEC_REVIEW.md` and must fix `PLAN.md`, `TEST_CASES.md` and the RED tests. The loop repeats until the CRITIC approves (`SPEC_STATUS: APPROVED`) or `MC_MAX_ATTEMPTS` / `-max-attempts` rounds are used up. Each retry prints:

```text
[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 2/3)...
```

Exit code `2` only occurs when the attempts are exhausted. Tokens from every round (including one interrupted by quota) are added to the run total.

### Resuming Execution & Connecting Existing Demands

You can resume interrupted runs or connect existing demands:

- `-resume`: Restores the exact execution state saved in `.makercycle/state/RESUME.env` when a run is paused (e.g. on quota limits).
- `-stage <1..5>`: Starts execution at any stage (1 to 5).
- `-demand <file.md>`: Connects an existing demand from `<docsDir>/demands/` (e.g. `-demand 2026-11-17-feature.md`) without creating a duplicate record.

### 4. Try different model combinations

Edit `.makercycle/config/models.env` or override with environment variables (see full details in the [Model Customization](#model-customization) section):

```bash
MC_MAKER_MODEL="GPT-OSS 120B (Medium)" go run . -task "Add cursor pagination"
```

Each agent has `MC_<AGENT>_BIN`, `_MODEL` and `_ARGS` (extra CLI flags, e.g. permission flags needed for headless mode). `--dangerously-skip-permissions` disables permission prompts: use it only in trusted repositories.

## Model Customization

Default CLI clients, models, and parameters for each agent are centralized and customized in:

📁 **`.makercycle/config/models.env`**

This file is automatically loaded on `makercycle` startup and can be committed to the repository to share project defaults.

### Configuration Variables per Agent

Each agent has dedicated environment variables to configure its CLI binary (`_BIN`), model (`_MODEL`), extra CLI arguments (`_ARGS`), and skill (`_SKILL` when applicable):

| Stage | Agent | Client / Binary | Model | Extra Arguments | Skill |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 0 | `SCOUT` | `MC_SCOUT_BIN` | `MC_SCOUT_MODEL` | `MC_SCOUT_ARGS` | — |
| 1 | `PLANNER` | `MC_PLANNER_BIN` | `MC_PLANNER_MODEL` | `MC_PLANNER_ARGS` | `MC_PLANNER_SKILL` |
| 2 | `CRITIC` | `MC_CRITIC_BIN` | `MC_CRITIC_MODEL` | `MC_CRITIC_ARGS` | `MC_CRITIC_SKILL` |
| 3 | `MAKER` | `MC_MAKER_BIN` | `MC_MAKER_MODEL` | `MC_MAKER_ARGS` | — |
| 4B | `AUDITOR` | `MC_AUDITOR_BIN` | `MC_AUDITOR_MODEL` | `MC_AUDITOR_ARGS` | `MC_AUDITOR_SKILL` |
| 5 | `GATEKEEPER` | `MC_GATEKEEPER_BIN` | `MC_GATEKEEPER_MODEL` | `MC_GATEKEEPER_ARGS` | — |

In addition to agents, `.makercycle/config/models.env` manages global loop settings:
- `MC_MAX_ATTEMPTS`: Maximum attempts in the correction loop before circuit breaker trips (default: `3`).
- `MC_DEFAULT_DOCS_DIR`: Default documentation directory (default: `docs`).

### Authentication & OmniRoute Fallback

By default (`MC_AUTH_MODE="api_key"`, set in `.makercycle/config/models.env`), agents authenticate with provider API keys that live **exclusively** in `.makercycle/config/project.env` (git-ignored, written with `0600` permissions), isolating costs per project:
- `ANTHROPIC_API_KEY`: Anthropic key (`MC_API_KEY` is accepted as an alias).
- `OPENAI_API_KEY`: OpenAI key.
- `GEMINI_API_KEY`: Google Gemini key.

All configured keys are injected into every agent subprocess. The `api_key` pre-flight check requires at least one of them (environment variables take precedence over the file). `makercycle` prints a warning if any API key is found in the versioned `.makercycle/config/models.env`.

When token or quota limits are reached, `makercycle` can transparently route requests through `OmniRoute`:
- `MC_OMNIROUTE_ENABLED`: Enables automatic OmniRoute fallback when set to `true` (default: `false`).
- `MC_OMNIROUTE_URL`: OmniRoute server address (default: `http://localhost:20128`).
- `MC_OMNIROUTE_MODEL`: Model routed via OmniRoute (default: `claude-sonnet-4-6`).

### Cross-Auditing (Multi-LLM)

Use up to 5 different models across the stages. It is strongly recommended to use at least 2 distinct providers — e.g. Anthropic for the MAKER and OpenAI/Gemini for the GATEKEEPER — to avoid confirmation bias when auditing the code. Configure each stage through `MC_<AGENT>_BIN` / `MC_<AGENT>_MODEL` in `.makercycle/config/models.env`, and store the matching provider keys in `.makercycle/config/project.env`.

### How to Customize

Configuration precedence is: **CLI Flags > Environment Variables > `.makercycle/config/models.env` > Hardcoded defaults**.

1. **Edit `.makercycle/config/models.env` directly** (recommended for persistent project defaults):
   ```env
   # Example: customize Stage 3 and Stage 5 models
   MC_MAKER_MODEL="Gemini 3.8 Flash (High)"
   MC_GATEKEEPER_MODEL="Gemini 3.8 Flash (High)"
   ```

2. **Override via environment variables** (ideal for a single run without touching config files):
   ```bash
   MC_MAKER_MODEL="GPT-OSS 120B (Medium)" go run . -task "Add cursor pagination"
   ```

3. **Override via CLI flags**:
   ```bash
   go run . -maker-model "Gemini 3.8 Flash (High)" -gatekeeper-model "Gemini 3.8 Flash (High)" -task "Add cursor pagination"
   ```

## Output layout

```text
<docs>/
├── PROJECT_CONTEXT.md     # project context (Stage 0)
├── demands/               # one file per demand, linked to its execution log
│   └── INDEX.md           # | Data | Stack | Demanda | Arquivo | Status |
├── logs/                  # one full log per run: YYYY-MM-DD_HHMMSS-<slug>.log
└── references/            # reference material (created by -init / -setup)
.makercycle/
├── agents/                # agent contracts (00_SCOUT.md ... 05_GATEKEEPER.md)
├── config/                # models.env (versioned), project.env (git-ignored)
├── state/                 # transient artifacts: PLAN, TEST_CASES, DECISION, ... (git-ignored)
└── telemetry/             # CSV metrics (header written on first use, then append-only)
    ├── runs.csv           # one row per run: attempts, tokens in/out/total, final status
    ├── events.csv         # one row per stage/agent execution: tokens, duration, attempt (joins runs.csv by RunID)
    └── TELEMETRY.md       # human-readable report generated from both CSVs (do not edit)
```

Logs contain the prompt and the full response of every agent, the complete test/linter output, the generated review/security/decision reports, durations, token counts and the final exit code.

`TELEMETRY.md` is regenerated at the end of every run from `runs.csv` and `events.csv`. It starts with the KPIs (total runs, success rate, accumulated tokens overall and per stage E1 to E5, average duration) followed by the run history, newest first, in a table with the header `| Data | RunID | Status | Demanda | Stack | Tentativas | Duração | Tokens Totais | Tokens E1 PLAN | ... | Tokens E5 GATE |`. If the report cannot be generated, only a warning is printed and the run is not interrupted.

## Exit codes

Stages 1 and 2 run as an autonomous loop (see [Autonomous specification loop](#autonomous-specification-loop)). Exit code `2` only happens when the CRITIC still answers `SPEC_STATUS: REVISION_REQUIRED` (or does not write the review) after `MC_MAX_ATTEMPTS` rounds; fix the plan and start again from Stage 1.

| Code | Meaning |
| :--- | :--- |
| `0` | `APPROVE_MERGE` (or a successful `-init`/`-setup`) |
| `1` | Fatal error or circuit breaker tripped |
| `2` | Specification blocked by the CRITIC (Stage 2) after exhausting `MC_MAX_ATTEMPTS` |
| `3` | `AMEND_TEST` — a Stage 1 test needs fixing |
| `4` | `ESCALATE_PLANNER` |
| `5` | Quota or token limit reached (state saved in `RESUME.env`, resumable with `-resume`) |

## Tests

```bash
go test -v -race ./... && go vet ./... && go build -o makercycle .
```

## License

Distributed under the **Apache License 2.0**. See `LICENSE`.
