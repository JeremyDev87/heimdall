# Heimdall

Heimdall is an evidence-first, deterministic evaluator for trusted local agent harnesses.
It freezes a target, executes a declared command in a disposable copy, verifies explicit
artifacts, seals content-light evidence, and reduces hard gates to one of four states.

> **Status:** Go MVP implementation. The bundled corpus is synthetic; no real agent harness has
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

Requirements: Go 1.26 or newer. Heimdall itself has no Python or `uv` runtime dependency.
A target command may independently require the interpreter declared in its manifest.

```bash
go build -trimpath -o ./bin/heimdall ./cmd/heimdall
./bin/heimdall validate fixtures/pass/eval.yaml
./bin/heimdall evaluate fixtures/pass/eval.yaml --out /tmp/heimdall-pass
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

## Security and platform boundary

`trusted-local` means the target is owner-controlled code. On supported hosted runtimes,
Heimdall uses a temporary copy, argv-only subprocess execution, a reduced environment, process-
group timeout cleanup, and source no-write verification. **This is not an OS sandbox:** network
access and arbitrary host writes are not prevented. Do not evaluate adversarial or third-party
code with this runner. Such targets require a future container or host-enforced sandbox backend.

The hosted runner is verified on macOS arm64. Linux amd64 has cross-build evidence and CI coverage,
but remains runtime-unverified until hosted process and timeout evidence is collected.
Hermes profiles are not used as a sandbox, and the bundled Skill is not installed automatically.
Human or host control retains final acceptance and every external state transition.

## Regression and migration corpus

The fixed corpus covers evidence-backed PASS, PASS-looking self-report with missing evidence,
missing receipts, writes outside the disposable target, and injection-like text treated as data.
Additional Go tests cover source mutation, non-zero exits, timeout descendant cleanup, schema
closure, output containment, duplicate keys, symlink rejection, and credential redaction.

`testdata/oracle/v1/ledger.json` records the canonical Python MVP results at Git revision
`87cbef5dc9bdc48e572e922b54a4eef452816ebd`. Go tests compare every bundled fixture against that
frozen oracle; the legacy Python product implementation is not required at test or runtime.

```bash
test -z "$(gofmt -l .)"
go vet ./...
go test ./... -count=1
go test -race ./... -count=1
go build -trimpath -o /tmp/heimdall ./cmd/heimdall
```

## Repository layout

```text
cmd/heimdall/        CLI entrypoint
internal/            contract, runner, reducer, report, and deterministic utilities
schemas/             strict v1 input/evidence/report contracts
policies/            versioned scoring policy artifacts
fixtures/            fixed adversarial and benign corpus
testdata/oracle/     frozen cross-language parity receipts
skill/heimdall/      distributable Hermes Skill; not profile-installed
```

## Deferred work

A first real target must be selected and frozen before Heimdall can claim product-level utility.
An optional evidence-only semantic reviewer may be considered only after deterministic gaps are
measured against a human-reviewed corpus. Container isolation, GitHub integration, publication,
deployment, automatic approval, and numeric scoring are outside this MVP.
