# Heimdall

Heimdall is an evidence-first, deterministic evaluator for trusted local agent harnesses.
It freezes a target, executes a declared command in a disposable copy, verifies explicit
artifacts, seals content-light evidence, and reduces hard gates to one of four states.

> **Status:** Go MVP implementation. The published `ddalggak` adapter has exact-revision local Darwin
> and hosted Ubuntu receipts. The hosted receipt passed on merge commit
> `7dd568511b5e37ee60ccbd5f4fe7e2f38a30debb` in
> [main run 30792274162](https://github.com/JeremyDev87/heimdall/actions/runs/30792274162).
> The source is licensed under MIT. There is no immutable tag or binary release yet, so a source
> checkout must not be presented as a stable installation channel.

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
VERSION=dev
COMMIT="$(git rev-parse HEAD)"
git diff --quiet --ignore-submodules -- || COMMIT="${COMMIT}-dirty"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
go build -trimpath -ldflags="-X github.com/JeremyDev87/heimdall/internal/cli.Version=${VERSION} -X github.com/JeremyDev87/heimdall/internal/cli.Commit=${COMMIT} -X github.com/JeremyDev87/heimdall/internal/cli.BuildDate=${BUILD_DATE}" -o ./bin/heimdall ./cmd/heimdall
./bin/heimdall version
./bin/heimdall validate fixtures/pass/eval.yaml
./bin/heimdall evaluate fixtures/pass/eval.yaml --out /tmp/heimdall-pass
```

For a new owner-controlled command harness, initialize a fail-closed scaffold from the harness root:

```bash
/path/to/heimdall init --preset command-artifact --target . -- ./scripts/verify-harness.sh
# Review .heimdall-eval.yaml, .heimdall-policy.yaml, and .heimdall/verify-harness.sh.
/path/to/heimdall check
```

`init` never learns expected evidence from a first run and never overwrites a differing scaffold.
The supplied command must itself verify the harness outcome; a command that merely exits successfully
is not an artifact oracle. `check` validates and evaluates with the existing four-state reducer, writes
the three report artifacts outside the target by default, and prints their resolved directory.

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

The hosted runner is verified on GitHub-hosted macOS and Ubuntu at merge commit
`7dd568511b5e37ee60ccbd5f4fe7e2f38a30debb`. The regular Ubuntu/macOS CI lanes passed in
[CI run 30792274120](https://github.com/JeremyDev87/heimdall/actions/runs/30792274120). The separate
pinned Linux real-target lane passed in
[run 30792274162](https://github.com/JeremyDev87/heimdall/actions/runs/30792274162); its artifact records
the exact `ddalggak` revision, two-run semantic digest agreement, `no_write=true`, and
`outside_workspace_write=false`. Cross-build success remains distinct from hosted runtime evidence.
Hermes profiles are not used as a sandbox, and the bundled Skill is not installed automatically.
Human or host control retains final acceptance and every external state transition.

## First trusted-local pilot

The published real-target pilot used Heimdall commit
`27c0aa105d7216bdc8d67ee3f544e3459422d7d0` against `ddalggak` commit
`89868c05ca781365701362db08666bca503901b2`. The adapter invokes ddalggak's existing deterministic
29-scenario readiness evaluator and writes `ok\n` only after that evaluator exits successfully.

The target-local policy is byte-identical to `policies/harness-readiness-v1.yaml`, with SHA-256
`2895584af90ffd13f9eebc84a3fe9334f78c4a9efcbcabff03976072eb7047f9`. Two clean positive
evaluations produced the same evidence digest
`d172d008de8152b7219c3f3b661219f1c3d265015936cc34ae5b069907cd1c98` and report digest
`969e6a8e589bec08aee40ffdf0ee71b67677e37796026fb3092a702f8618c15b`, with target no-write
evidence true. A fixture-tamper probe exited `1` and reduced to `FAIL` with `command_failed` and
`required_evidence_missing`; it could not reuse or emit a stale pass artifact.

This is exact published-revision local evidence; the separate hosted Linux result is documented above.
The evaluated target digest
`5a2330e555ad4f42a4eb02cf51c4fe9ad90ab2f4fc90220c753d53a9223d1df2` binds the exact
Heimdall-evaluated target snapshot: relative paths, file/directory kinds, and regular-file bytes.
It excludes `.git`, `.venv`, `.pytest_cache`, `.ruff_cache`, and `__pycache__`, and does not seal
filesystem metadata such as mode or timestamps. It proves one bounded repository can use the
existing contract; it does not prove universal target coverage or provide an adversarial-code
sandbox.

## Linux real-target gate

The hosted lane checks out the current exact Heimdall revision and fetches the pinned public
`ddalggak` revision above into the runner's temporary directory. It validates the manifest, evaluates
the target twice, and fails unless the state is `PASS`, process/check receipts pass, semantic digests
repeat, the target remains unchanged, and Heimdall's bounded `outside_workspace_write` receipt is
false. That receipt covers Heimdall's runner-workspace boundary signals; it does not observe arbitrary
host writes and does not turn `trusted-local` execution into a security sandbox. The workflow uploads
both three-artifact runs plus semantic and always-written CI execution receipts.

The same gate can be reproduced from a clean ddalggak checkout without changing either repository:

```bash
bash scripts/verify-ddalggak-real-target.sh \
  /path/to/ddalggak /tmp/heimdall-ddalggak-real-target \
  89868c05ca781365701362db08666bca503901b2 "$(git rev-parse HEAD)"
```

The verifier builds Heimdall from the checkout containing the script. The hosted workflow additionally
requires that checkout to be clean, so an arbitrary caller-supplied executable cannot forge a PASS.

The Darwin semantic digests are not hard-coded as Linux expectations. The Linux lane establishes
repeatability by comparing two evaluations on the same exact hosted runtime and preserves the
observed digests in `receipt.json`.

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
templates/           embedded, reviewable onboarding scaffold templates
```

## License

Heimdall source code is licensed under the [MIT License](LICENSE). This license governs source reuse;
it does not mean that a versioned binary, checksum, package, or stable installation channel has been
published. Until an immutable release exists, build from an exact source revision and preserve the
runtime provenance reported by `heimdall version`.

## Deferred work

Producing an immutable, checksum-verified binary release is the next public distribution gate. A
reusable GitHub Action remains dependent on that release. An optional
evidence-only semantic reviewer may be considered only after deterministic gaps are measured against
a human-reviewed corpus. Container isolation, publication, deployment, automatic approval, and
numeric scoring remain outside this MVP.
