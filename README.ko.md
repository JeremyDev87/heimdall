# Heimdall

**언어 / Language:** [English](README.md) · **[한국어](README.ko.md)** · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [Español](README.es.md)

Heimdall은 신뢰된 로컬 에이전트 하네스를 위한 **증거 우선·결정론적 평가기**입니다. 대상을 고정하고, 선언된 명령을 임시 복사본에서 실행하며, 명시된 산출물을 검증하고, 내용이 제한된 증거를 봉인한 뒤, hard gate를 네 가지 상태 중 하나로 축약합니다.

> **기본 문서 언어:** 영어입니다. 이 파일은 영어 기본 README의 번역본입니다.
>
> **현재 상태:** Go MVP이며 immutable·checksum 검증 바이너리 release를 제공합니다. `v0.1.0`은 정확한 `main` commit [`1cc04368`](https://github.com/JeremyDev87/heimdall/commit/1cc04368aebe25d459cc65796855a9f3e9ce3338)을 대상으로 합니다. draft build, 원격 byte 비교, tokenless runtime, immutable publication gate는 [workflow run 30908286471](https://github.com/JeremyDev87/heimdall/actions/runs/30908286471)에서 통과했습니다.

## 왜 Heimdall인가

에이전트나 하네스가 `PASS`, `approved`, `read_only`라고 말하는 것만으로는 독립 증거가 아닙니다. Heimdall은 verdict를 만들기 전에 process exit, 산출물 digest, source invariance, workspace 경계 write, policy identity를 기록합니다.

권위 있는 경로는 결정론적입니다. 이 저장소에는 LLM grader, 숫자 점수, auto-fixer, dashboard, approval service, universal adapter가 없습니다.

## Immutable 바이너리 release

안정적인 배포 채널은 immutable [`v0.1.0` release](https://github.com/JeremyDev87/heimdall/releases/tag/v0.1.0)입니다. 정확히 다음 세 asset을 포함합니다.

- `heimdall_0.1.0_linux_amd64.tar.gz`
- `heimdall_0.1.0_darwin_arm64.tar.gz`
- `checksums.txt`

각 archive에는 `heimdall`, `LICENSE`, `README.md`만 들어 있습니다. `v0.1.0`은 이 문서 갱신 전에 태그된 immutable release이므로 archive 안의 `README.md`는 태그 시점의 게시 전 snapshot이며 아직 binary release가 없다고 적혀 있습니다. 현재 release 안내의 기준은 default branch의 최신 README입니다.

압축을 풀기 전에 checksum을 검증하고 runtime provenance를 확인하십시오.

```bash
ARCHIVE=heimdall_0.1.0_darwin_arm64.tar.gz
curl -fLO "https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/${ARCHIVE}"
curl -fLO https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/checksums.txt
grep "  ${ARCHIVE}\$" checksums.txt | shasum -a 256 --check
tar -xzf "${ARCHIVE}"
./heimdall version
```

Linux amd64에서는 `ARCHIVE=heimdall_0.1.0_linux_amd64.tar.gz`로 설정하십시오. 출력된 version은 `0.1.0`, commit은 `1cc04368aebe25d459cc65796855a9f3e9ce3338`, `build_date`는 비어 있지 않아야 합니다.

Workflow는 hosted release run에서 draft artifact와 원격 업로드 asset의 byte equality를 증명합니다. 다른 Go toolchain으로 만든 로컬 재빌드가 byte-identical하다는 보장은 없습니다. release의 권위는 published checksum입니다.

## 소스에서 빌드하기

요구사항: Go 1.26 이상. Heimdall 자체는 Python이나 `uv` runtime dependency가 없습니다. 단, target command는 manifest에 선언한 interpreter를 별도로 요구할 수 있습니다.

```bash
VERSION=dev
COMMIT="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain=v1 --untracked-files=normal --ignore-submodules=dirty)" ]; then
  COMMIT="${COMMIT}-dirty"
fi
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
go build -trimpath \
  -ldflags="-X github.com/JeremyDev87/heimdall/internal/cli.Version=${VERSION} -X github.com/JeremyDev87/heimdall/internal/cli.Commit=${COMMIT} -X github.com/JeremyDev87/heimdall/internal/cli.BuildDate=${BUILD_DATE}" \
  -o ./bin/heimdall ./cmd/heimdall
./bin/heimdall version
./bin/heimdall validate fixtures/pass/eval.yaml
./bin/heimdall evaluate fixtures/pass/eval.yaml --out /tmp/heimdall-pass
```

새 owner-controlled command harness는 다음처럼 fail-closed scaffold로 초기화할 수 있습니다.

```bash
/path/to/heimdall init --preset command-artifact --target . -- ./scripts/verify-harness.sh
/path/to/heimdall check
```

`init`은 첫 실행에서 기대 증거를 학습하지 않으며, 다른 scaffold를 덮어쓰지 않습니다. 제공된 command 자체가 harness 결과를 검증해야 합니다. 단순히 성공 exit하는 command는 artifact oracle이 아닙니다.

## MVP 아키텍처

```text
versioned eval manifest + policy
              │
              ▼
frozen target → trusted-local runner → deterministic checks
              │
              ▼
content-light evidence → hard-gate reducer → JSON + Markdown report
```

Manifest는 상대 target root, versioned policy, argv-only command, timeout, 결정론적 file check를 선언합니다.

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

지원 check 종류는 `file_exists`, `file_equals`, `path_absent`입니다. 알 수 없는 field, 중복 YAML key, path traversal, policy drift, malformed contract는 fail closed됩니다.

## 상태와 exit code

| 상태 | Exit | 의미 |
| --- | ---: | --- |
| `PASS` | 0 | 모든 필수 결정론적 gate 통과 |
| `FAIL` | 1 | verifier, command, no-write, workspace 경계 gate 실패 |
| `BLOCKED` | 2 | contract, policy, target 또는 실행 전제조건을 사용할 수 없음 |
| `INCONCLUSIVE` | 3 | 실행이 불완전하거나 필수 증거가 누락됨 |

Hard failure는 평균으로 상쇄할 수 없습니다. 이 결정론적 MVP에서는 model이 `failure_honesty`를 추측하지 않고 `N/A`로 둡니다.

## Evidence contract

평가는 정확히 세 산출물을 작성합니다.

- `evidence.json`: target/policy digest, process receipt, content digest, check receipt
- `report.json`: canonical state, reason code, criteria, semantic digest
- `report.md`: `report.json`의 결정론적 projection

Raw stdout, stderr, environment value, target content, credential, absolute target path는 report에 복사하지 않습니다. content digest와 byte size로만 표현합니다. semantic document에서 timestamp와 temporary path를 제외하므로 반복 실행을 비교할 수 있습니다.

## 보안 및 platform 경계

`trusted-local`은 target이 owner-controlled code라는 뜻입니다. 지원되는 hosted runtime에서 Heimdall은 temporary copy, argv-only subprocess, reduced environment, process-group timeout cleanup, source no-write 검증을 사용합니다.

**OS sandbox가 아닙니다:** network access와 임의 host write를 차단하지 않습니다. adversarial 또는 third-party code를 이 runner로 평가하지 마십시오. 그런 target에는 향후 container 또는 host-enforced sandbox backend가 필요합니다. 최종 acceptance와 모든 외부 상태 전이는 사람 또는 host가 통제합니다.

Cross-build 성공은 hosted runtime 증거와 별개입니다. 번들 Hermes Skill은 배포 가능하지만 자동 설치되지 않습니다.

## Real-target validation

첫 trusted-local pilot은 Heimdall `27c0aa105d7216bdc8d67ee3f544e3459422d7d0`으로 `ddalggak` `89868c05ca781365701362db08666bca503901b2`를 평가했습니다. 두 번의 clean positive evaluation에서 evidence digest `d172d008de8152b7219c3f3b661219f1c3d265015936cc34ae5b069907cd1c98`와 report digest `969e6a8e589bec08aee40ffdf0ee71b67677e37796026fb3092a702f8618c15b`가 반복되었고 target no-write 증거가 true였습니다. fixture-tamper probe는 exit `1`, `FAIL`, `command_failed` 및 `required_evidence_missing`가 되었습니다.

Hosted Linux real-target lane은 정확한 Heimdall revision과 고정된 `ddalggak` revision을 사용하고 manifest 검증, 두 번의 평가, `PASS`, receipt 통과, semantic digest 반복, target 불변, `outside_workspace_write=false`를 요구합니다. 이 제한된 receipt는 임의의 host write를 관찰하지 않으며 `trusted-local`을 OS sandbox로 만들지 않습니다. 재현 명령:

```bash
bash scripts/verify-ddalggak-real-target.sh \
  /path/to/ddalggak /tmp/heimdall-ddalggak-real-target \
  89868c05ca781365701362db08666bca503901b2 "$(git rev-parse HEAD)"
```

## 검증 및 regression corpus

고정 corpus는 증거가 있는 PASS, 증거가 빠진 PASS-looking self-report, missing receipt, disposable target 밖 write, injection-like text를 data로 취급하는 경우를 포함합니다. Go test는 source mutation, non-zero exit, timeout descendant cleanup, schema closure, output containment, duplicate key, symlink rejection, credential redaction도 검사합니다.

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
internal/            contract, runner, reducer, report, deterministic utility
schemas/             strict v1 input/evidence/report contract
policies/            versioned policy artifact
fixtures/            fixed adversarial/benign corpus
testdata/oracle/     frozen cross-language parity receipt
skill/heimdall/      distributable Hermes Skill; profile에 자동 설치하지 않음
templates/           embedded onboarding scaffold template
```

## License와 distribution 경계

Heimdall source code는 [MIT License](LICENSE)입니다. immutable `v0.1.0` binary release는 별도의 checksum 검증 distribution artifact입니다. source reuse, binary redistribution, stable installation channel 주장을 서로 혼동하지 마십시오.

## Deferred work

검증된 immutable release contract를 소비하는 reusable GitHub Action은 후속 작업입니다. Container isolation, automatic approval, deployment, numeric scoring은 이 MVP의 범위 밖입니다.
