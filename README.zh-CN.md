# Heimdall

**语言 / Language:** [English](README.md) · [한국어](README.ko.md) · **[简体中文](README.zh-CN.md)** · [日本語](README.ja.md) · [Español](README.es.md)

Heimdall 是面向可信本地 agent harness 的**证据优先、确定性评估器**。它会冻结目标，在一次性副本中执行声明的命令，验证明确的产物，封存内容受限的证据，并将硬门槛归约为四种状态之一。

> **默认文档语言：** 英语。其他文件是本文件的翻译。
>
> **当前状态：** Go MVP，已经提供不可变且经过 checksum 验证的二进制 release。`v0.1.0` 精确对应 `main` commit [`1cc04368`](https://github.com/JeremyDev87/heimdall/commit/1cc04368aebe25d459cc65796855a9f3e9ce3338)。draft build、远程 byte 比较、无 token runtime、不可变发布门槛均在 [workflow run 30908286471](https://github.com/JeremyDev87/heimdall/actions/runs/30908286471) 中通过。

## 为什么使用 Heimdall

agent 或 harness 输出 `PASS`、`approved`、`read_only` 并不是独立证据。Heimdall 会在生成 verdict 前记录进程退出码、产物 digest、源代码不变性、workspace 边界写入和 policy identity。

权威路径是确定性的。本仓库不包含 LLM grader、数字评分、自动修复器、dashboard、approval service 或 universal adapter。

## 不可变二进制 release

稳定分发渠道是不可变的 [`v0.1.0` release](https://github.com/JeremyDev87/heimdall/releases/tag/v0.1.0)，且只包含以下三个 asset：

- `heimdall_0.1.0_linux_amd64.tar.gz`
- `heimdall_0.1.0_darwin_arm64.tar.gz`
- `checksums.txt`

每个 archive 只包含 `heimdall`、`LICENSE` 和 `README.md`。解压前先验证 checksum，再检查 runtime provenance：

```bash
# Darwin arm64
curl -fLO https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/heimdall_0.1.0_darwin_arm64.tar.gz
curl -fLO https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/checksums.txt
shasum -a 256 --check checksums.txt
tar -xzf heimdall_0.1.0_darwin_arm64.tar.gz
./heimdall version
```

Linux amd64 请改为下载 `heimdall_0.1.0_linux_amd64.tar.gz`。输出的 version 必须是 `0.1.0`，commit 必须是 `1cc04368aebe25d459cc65796855a9f3e9ce3338`，并且必须存在 `build_date`。

workflow 证明 hosted release run 中 draft artifact 与远程上传 asset 的 byte equality。使用不同 Go toolchain 的本地重建不保证 byte-identical；release 的权威是已发布的 checksum。

## 从源码构建

要求：Go 1.26 或更高版本。Heimdall 本身没有 Python 或 `uv` runtime dependency，但 target command 可能需要 manifest 声明的 interpreter。

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

对于新的 owner-controlled command harness，可以初始化 fail-closed scaffold：

```bash
/path/to/heimdall init --preset command-artifact --target . -- ./scripts/verify-harness.sh
/path/to/heimdall check
```

`init` 不会从第一次运行中学习预期证据，也不会覆盖不同的 scaffold。提供的 command 必须自行验证 harness 结果；只返回成功 exit 的 command 不是 artifact oracle。

## MVP 架构

```text
versioned eval manifest + policy
              │
              ▼
frozen target → trusted-local runner → deterministic checks
              │
              ▼
content-light evidence → hard-gate reducer → JSON + Markdown report
```

Manifest 声明相对 target root、版本化 policy、argv-only command、timeout 和确定性的文件检查：

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

支持的 check 类型是 `file_exists`、`file_equals` 和 `path_absent`。未知字段、重复 YAML key、path traversal、policy drift 和 malformed contract 都会 fail closed。

## 状态和退出码

| 状态 | Exit | 含义 |
| --- | ---: | --- |
| `PASS` | 0 | 所有必需的确定性门槛通过 |
| `FAIL` | 1 | verifier、command、no-write 或 workspace 边界门槛失败 |
| `BLOCKED` | 2 | contract、policy、target 或执行前提不可用 |
| `INCONCLUSIVE` | 3 | 执行不完整或缺少必需证据 |

Hard failure 不能通过平均被抵消。这个确定性 MVP 不让模型猜测 `failure_honesty`，因此保持为 `N/A`。

## Evidence contract

一次评估只写入三个产物：

- `evidence.json`：target/policy digest、process receipt、content digest、check receipt；
- `report.json`：canonical state、reason code、criteria、semantic digest；
- `report.md`：`report.json` 的确定性投影。

Raw stdout、stderr、environment value、target content、credential 和绝对 target path 不会复制到报告中，只以 content digest 和 byte size 表示。semantic document 不含 timestamp 和临时路径，因此可比较重复运行。

## 安全与平台边界

`trusted-local` 表示 target 是 owner-controlled code。在支持的 hosted runtime 中，Heimdall 使用 temporary copy、argv-only subprocess、reduced environment、process-group timeout cleanup 和 source no-write verification。

**这不是 OS sandbox：** 它不会阻止 network access 或任意 host write。不要使用此 runner 评估 adversarial 或 third-party code；这类 target 需要未来的 container 或 host-enforced sandbox backend。最终 acceptance 和每一次外部状态转换都由人或 host 控制。

Cross-build 成功与 hosted runtime 证据是两类不同证据。随附的 Hermes Skill 可分发，但不会自动安装。

## Real-target validation

第一次 trusted-local pilot 使用 Heimdall `27c0aa105d7216bdc8d67ee3f544e3459422d7d0` 评估 `ddalggak` `89868c05ca781365701362db08666bca503901b2`。两次 clean positive evaluation 得到相同的 evidence digest `d172d008de8152b7219c3f3b661219f1c3d265015936cc34ae5b069907cd1c98` 和 report digest `969e6a8e589bec08aee40ffdf0ee71b67677e37796026fb3092a702f8618c15b`，target no-write 为 true。fixture-tamper probe 以 exit `1` 结束，并归约为 `FAIL`，原因是 `command_failed` 和 `required_evidence_missing`。

Hosted Linux real-target lane 使用精确的 Heimdall revision 和固定的 `ddalggak` revision，验证 manifest、执行两次评估，并要求 `PASS`、receipt 通过、semantic digest 重复、target 未改变以及 `outside_workspace_write=false`。该受限 receipt 不会观察任意 host write，也不会把 `trusted-local` 变成 OS sandbox。复现命令：

```bash
bash scripts/verify-ddalggak-real-target.sh \
  /path/to/ddalggak /tmp/heimdall-ddalggak-real-target \
  89868c05ca781365701362db08666bca503901b2 "$(git rev-parse HEAD)"
```

## 验证与 regression corpus

固定 corpus 覆盖有证据的 PASS、缺少证据但看似 PASS 的自报、missing receipt、disposable target 外写入，以及被当作数据处理的 injection-like text。Go tests 还覆盖 source mutation、non-zero exit、timeout descendant cleanup、schema closure、output containment、duplicate key、symlink rejection 和 credential redaction。

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
internal/            contract、runner、reducer、report 和确定性工具
schemas/             strict v1 input/evidence/report contract
policies/            版本化 policy artifact
fixtures/            固定的 adversarial/benign corpus
testdata/oracle/     冻结的跨语言 parity receipt
skill/heimdall/      可分发 Hermes Skill；不会自动安装到 profile
templates/           embedded onboarding scaffold template
```

## License 与分发边界

Heimdall source code 使用 [MIT License](LICENSE)。不可变的 `v0.1.0` binary release 是单独的、经过 checksum 验证的 distribution artifact。不要混淆 source reuse、binary redistribution 和 stable installation channel 的声明。

## 后续工作

消费已验证 immutable release contract 的 reusable GitHub Action 属于后续工作。Container isolation、automatic approval、deployment 和 numeric scoring 不在本 MVP 范围内。
