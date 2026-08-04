# Heimdall

**Language / 언어 / 语言 / 言語 / Idioma:** **[English](README.md)** · [한국어](README.ko.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [Español](README.es.md)

Heimdall is an evidence-first, deterministic evaluator for trusted-local agent harnesses. It freezes a target, executes a declared command in a disposable copy, verifies explicit artifacts, seals content-light evidence, and reduces hard gates to one of four states.

> **Default documentation language:** English. The other files are translations of this document.
>
> **Current status:** Go MVP with an immutable, checksum-verified binary release. `v0.1.0` targets the exact `main` commit [`1cc04368`](https://github.com/JeremyDev87/heimdall/commit/1cc04368aebe25d459cc65796855a9f3e9ce3338). The release workflow passed its draft-build, remote byte-compare, tokenless runtime, and immutable-publication gates in [run 30908286471](https://github.com/JeremyDev87/heimdall/actions/runs/30908286471).

## Why Heimdall

An agent or harness saying `PASS`, `approved`, or `read_only` is not independent evidence. Heimdall records process exits, artifact digests, source invariance, workspace-boundary writes, and policy identity before producing a verdict.

The authoritative path is deterministic. This repository does not contain an LLM grader, numeric score, auto-fixer, dashboard, approval service, or universal adapter.

## Immutable binary release

The stable distribution channel is the immutable [`v0.1.0` release](https://github.com/JeremyDev87/heimdall/releases/tag/v0.1.0). It contains exactly:

- `heimdall_0.1.0_linux_amd64.tar.gz`
- `heimdall_0.1.0_darwin_arm64.tar.gz`
- `checksums.txt`

Each archive contains only `heimdall`, `LICENSE`, and `README.md`. Verify the checksum before extracting, then verify runtime provenance:

```bash
ARCHIVE=heimdall_0.1.0_darwin_arm64.tar.gz
curl -fLO "https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/${ARCHIVE}"
curl -fLO https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/checksums.txt
grep "  ${ARCHIVE}\$" checksums.txt | shasum -a 256 --check
tar -xzf "${ARCHIVE}"
./heimdall version
```

For Linux amd64, set `ARCHIVE=heimdall_0.1.0_linux_amd64.tar.gz` instead. The reported version must be `0.1.0`, the commit must be `1cc04368aebe25d459cc65796855a9f3e9ce3338`, and `build_date` must be present.

The workflow proves byte equality between the draft artifacts and the assets uploaded by that hosted release run. A local rebuild made with a different Go toolchain is not promised to be byte-identical; use the published checksums as the release authority.

## Build from source

Requirements: Go 1.26 or newer. Heimdall itself has no Python or `uv` runtime dependency. A target command may independently require the interpreter declared in its manifest.

```bash
VERSION=dev
COMMIT="$(git rev-parse HEAD)"
git diff --quiet --ignore-submodules -- || COMMIT="${COMMIT}-dirty"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
go build -trimpath \
  -ldflags="-X github.com/JeremyDev87/heimdall/internal/cli.Version=${VERSION} -X github.com/JeremyDev87/heimdall/internal/cli.Commit=${COMMIT} -X github.com/JeremyDev87/heimdall/internal/cli.BuildDate=${BUILD_DATE}" \
  -o ./bin/heimdall ./cmd/heimdall
./bin/heimdall version
./bin/heimdall validate fixtures/pass/eval.yaml
./bin/heimdall evaluate fixtures/pass/eval.yaml --out /tmp/heimdall-pass
```

For a new owner-controlled command harness, initialize a fail-closed scaffold from the harness root:

```bash
/path/to/heimdall init --preset command-artifact --target . -- ./scripts/verify-harness.sh
/path/to/heimdall check
```

`init` never learns expected evidence from a first run and never overwrites a differing scaffold. The supplied command must itself verify the harness outcome; a command that merely exits successfully is not an artifact oracle.

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

A manifest declares a relative target root, a versioned policy, an argv-only command, a timeout, and deterministic file checks:

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

Supported check kinds are `file_exists`, `file_equals`, and `path_absent`. Unknown fields, duplicate YAML keys, path traversal, policy drift, and malformed contracts fail closed.

## States and exit codes

| State | Exit | Meaning |
| --- | ---: | --- |
| `PASS` | 0 | Every required deterministic gate passed. |
| `FAIL` | 1 | A verifier, command, no-write, or workspace-boundary gate failed. |
| `BLOCKED` | 2 | The contract, policy, target, or execution prerequisite was unavailable. |
| `INCONCLUSIVE` | 3 | Execution completed incompletely or required evidence was missing. |

Hard failures cannot be averaged away. `failure_honesty` remains `N/A` in this deterministic MVP rather than being guessed by a model.

## Evidence contract

An evaluation writes exactly three artifacts:

- `evidence.json`: target/policy digests, process receipt, content digests, and check receipts;
- `report.json`: canonical state, reason codes, criteria, and semantic digest;
- `report.md`: a deterministic projection of `report.json`.

Raw stdout, stderr, environment values, target content, credentials, and absolute target paths are not copied into these reports. They are represented only by content digests and byte sizes. Semantic documents omit timestamps and temporary paths so repeat runs remain comparable.

## Security and platform boundary

`trusted-local` means the target is owner-controlled code. On supported hosted runtimes, Heimdall uses a temporary copy, argv-only subprocess execution, a reduced environment, process-group timeout cleanup, and source no-write verification.

**This is not an OS sandbox:** network access and arbitrary host writes are not prevented. Do not evaluate adversarial or third-party code with this runner. Such targets require a future container or host-enforced sandbox backend. Human or host control retains final acceptance and every external state transition.

Cross-build success remains distinct from hosted runtime evidence. The bundled Hermes Skill is distributable but is not installed automatically.

## Real-target validation

The first trusted-local pilot used Heimdall commit `27c0aa105d7216bdc8d67ee3f544e3459422d7d0` against `ddalggak` commit `89868c05ca781365701362db08666bca503901b2`. Two clean positive evaluations produced the same evidence digest `d172d008de8152b7219c3f3b661219f1c3d265015936cc34ae5b069907cd1c98` and report digest `969e6a8e589bec08aee40ffdf0ee71b67677e37796026fb3092a702f8618c15b`, with target no-write evidence true. A fixture-tamper probe exited `1` and reduced to `FAIL` with `command_failed` and `required_evidence_missing`.

The hosted Linux real-target lane checks out the exact Heimdall revision, fetches the pinned `ddalggak` revision, validates the manifest, evaluates twice, and requires `PASS`, passing receipts, repeated semantic digests, unchanged target content, and `outside_workspace_write=false`. The bounded receipt does not observe arbitrary host writes and does not turn `trusted-local` execution into a security sandbox. Reproduce it from a clean checkout with:

```bash
bash scripts/verify-ddalggak-real-target.sh \
  /path/to/ddalggak /tmp/heimdall-ddalggak-real-target \
  89868c05ca781365701362db08666bca503901b2 "$(git rev-parse HEAD)"
```

## Verification and regression corpus

The fixed corpus covers evidence-backed PASS, PASS-looking self-report with missing evidence, missing receipts, writes outside the disposable target, and injection-like text treated as data. Additional Go tests cover source mutation, non-zero exits, timeout descendant cleanup, schema closure, output containment, duplicate keys, symlink rejection, and credential redaction.

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
policies/            versioned policy artifacts
fixtures/            fixed adversarial and benign corpus
testdata/oracle/     frozen cross-language parity receipts
skill/heimdall/      distributable Hermes Skill; not profile-installed
templates/           embedded onboarding scaffold templates
```

## License and distribution boundary

Heimdall source code is licensed under the [MIT License](LICENSE). The immutable `v0.1.0` binary release is a separate, checksum-verified distribution artifact. Source reuse, binary redistribution, and stable installation-channel claims must not be conflated.

## Deferred work

A reusable GitHub Action remains a follow-up that consumes the proven immutable release contract. Container isolation, automatic approval, deployment, and numeric scoring remain outside this MVP.
