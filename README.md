# Heimdall

Heimdall is an evidence-first, deterministic evaluator for trusted local agent harnesses.
It freezes a target, executes a declared command in a disposable copy, verifies explicit
artifacts, seals content-light evidence, and reduces hard gates to one of four states.

> **Status:** MVP implementation. The bundled corpus is synthetic; no real agent harness has
> yet been selected as the first product target.

## Why Heimdall

An agent or harness saying `PASS`, `approved`, or `read_only` is not independent evidence.
Heimdall records process exits, artifact digests, source invariance, workspace-boundary writes,
and policy identity before producing a verdict.

## MVP architecture

```text
versioned eval manifest + policy
              │
              ▼
frozen target → trusted-local runner → deterministic checks
              │
              ▼
content-light evidence → hard-gate reducer → JSON + Markdown report
```

The authoritative path is deterministic. The repository does not contain an LLM grader,
numeric score, auto-fixer, dashboard, approval service, or universal adapter.

## Quick start

Requirements: Python 3.11 or newer and [uv](https://docs.astral.sh/uv/).

```bash
uv sync
uv run heimdall validate fixtures/pass/eval.yaml
uv run heimdall evaluate fixtures/pass/eval.yaml --out /tmp/heimdall-pass
```

A manifest declares a relative target root, a versioned policy, an argv-only command, a
timeout, and deterministic file checks:

```yaml
schema_version: "1.0"
target:
  id: example-harness
  root: target
policy:
  id: harness-readiness
  version: "1"
  path: ../../policies/harness-readiness-v1.yaml
isolation: trusted-local
command:
  argv: [python3, run.py]
  timeout_seconds: 10
checks:
  - id: result
    kind: file_equals
    path: result.txt
    expected: "ok\n"
```

Supported check kinds are `file_exists`, `file_equals`, and `path_absent`. Unknown fields,
duplicate YAML keys, path traversal, policy drift, and malformed contracts fail closed.

## States and exit codes

| State | Exit | Meaning |
| --- | ---: | --- |
| `PASS` | 0 | Every required deterministic gate passed. |
| `FAIL` | 1 | A verifier, command, no-write, or workspace-boundary gate failed. |
| `BLOCKED` | 2 | The contract, policy, target, or execution prerequisite was unavailable. |
| `INCONCLUSIVE` | 3 | Execution completed incompletely or required evidence was missing. |

Hard failures cannot be averaged away. `failure_honesty` remains `N/A` in this deterministic
MVP rather than being guessed by a model.

## Evidence contract

An evaluation writes exactly three artifacts:

- `evidence.json`: target/policy digests, process receipt, content digests, and check receipts;
- `report.json`: canonical state, reason codes, criteria, and semantic digest;
- `report.md`: a deterministic projection of `report.json`.

Raw stdout, stderr, environment values, target content, credentials, and absolute target paths
are not copied into these reports. They are represented only by content digests and byte sizes.
The semantic documents omit timestamps and temporary paths so repeat runs remain comparable.

## Security boundary

`trusted-local` means the target is owner-controlled code. Heimdall currently uses a temporary
copy, argv-only subprocess execution, a reduced environment, a timeout, and source no-write
verification. **This is not an OS sandbox:** network access and arbitrary host writes are not
prevented. Do not evaluate adversarial or third-party code with this runner. Such targets require
a future container or host-enforced sandbox backend and must be treated as `BLOCKED` today.

Hermes profiles are not used as a sandbox, and the bundled Skill is not installed automatically.
Human or host control retains final acceptance and every external state transition.

## Regression corpus

The fixed corpus covers:

- valid evidence-backed PASS;
- PASS-looking self-report with missing evidence;
- missing receipt;
- a write outside the allowed disposable target directory;
- evaluator-directed prompt-injection text treated only as data.

Additional tests cover source mutation, non-zero exits, timeouts, copied-root stability,
report-schema closure, output-path containment, duplicate keys, and credential redaction.

```bash
uv run ruff check .
uv run python -m compileall -q src tests
uv run pytest -q
```

## Repository layout

```text
src/heimdall/        deterministic core and CLI
schemas/             strict v1 input/evidence/report contracts
policies/            versioned scoring policy artifacts
fixtures/            fixed adversarial and benign corpus
skill/heimdall/       distributable Hermes Skill; not profile-installed
tests/               contract, reducer, mutation, privacy, and CLI tests
```

## Deferred work

A first real target must be selected and frozen before Heimdall can claim product-level utility.
An optional evidence-only semantic reviewer may be considered only after deterministic gaps are
measured against a human-reviewed corpus. Docker/container isolation, GitHub integration,
publication, deployment, automatic approval, and numeric scoring are outside this MVP.
