# Heimdall

**言語 / Language:** [English](README.md) · [한국어](README.ko.md) · [简体中文](README.zh-CN.md) · **[日本語](README.ja.md)** · [Español](README.es.md)

Heimdall は、信頼されたローカル agent harness のための**証拠優先・決定論的評価器**です。対象を固定し、宣言されたコマンドを使い捨てコピー上で実行し、明示された成果物を検証し、内容を限定した証拠を封印したうえで、hard gate を4つの状態のいずれかに縮約します。

> **既定の文書言語:** 英語です。このファイルは英語版の既定 README の翻訳です。
>
> **現在の状態:** Go MVP で、immutable かつ checksum 検証済みの binary release を提供しています。`v0.1.0` は `main` の正確な commit [`1cc04368`](https://github.com/JeremyDev87/heimdall/commit/1cc04368aebe25d459cc65796855a9f3e9ce3338) を対象にします。draft build、remote byte 比較、tokenless runtime、immutable publication gate は [workflow run 30908286471](https://github.com/JeremyDev87/heimdall/actions/runs/30908286471) で成功しました。

## なぜ Heimdall なのか

agent や harness が `PASS`、`approved`、`read_only` と報告しても、それだけでは独立した証拠ではありません。Heimdall は verdict を生成する前に、process exit、artifact digest、source invariance、workspace 境界への write、policy identity を記録します。

権威ある経路は決定論的です。この repository には LLM grader、数値スコア、auto-fixer、dashboard、approval service、universal adapter はありません。

## Immutable binary release

安定した配布チャネルは immutable な [`v0.1.0` release](https://github.com/JeremyDev87/heimdall/releases/tag/v0.1.0) です。asset は次の3つだけです。

- `heimdall_0.1.0_linux_amd64.tar.gz`
- `heimdall_0.1.0_darwin_arm64.tar.gz`
- `checksums.txt`

各 archive には `heimdall`、`LICENSE`、`README.md` だけが含まれます。展開前に checksum を検証し、その後 runtime provenance を確認してください。

```bash
ARCHIVE=heimdall_0.1.0_darwin_arm64.tar.gz
curl -fLO "https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/${ARCHIVE}"
curl -fLO https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/checksums.txt
grep "  ${ARCHIVE}\$" checksums.txt | shasum -a 256 --check
tar -xzf "${ARCHIVE}"
./heimdall version
```

Linux amd64 では `ARCHIVE=heimdall_0.1.0_linux_amd64.tar.gz` に設定してください。表示される version は `0.1.0`、commit は `1cc04368aebe25d459cc65796855a9f3e9ce3338`、`build_date` は存在していなければなりません。

workflow は hosted release run における draft artifact と remote asset の byte equality を証明します。異なる Go toolchain で作ったローカル再ビルドが byte-identical になる保証はありません。release の権威は公開 checksum です。

## ソースからビルドする

要件: Go 1.26 以上。Heimdall 自体に Python や `uv` の runtime dependency はありません。ただし target command は manifest に宣言された interpreter を別途必要とする場合があります。

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

新しい owner-controlled command harness は fail-closed scaffold として初期化できます。

```bash
/path/to/heimdall init --preset command-artifact --target . -- ./scripts/verify-harness.sh
/path/to/heimdall check
```

`init` は初回実行から期待される証拠を学習せず、異なる scaffold を上書きしません。指定した command 自体が harness の結果を検証する必要があります。成功 exit するだけの command は artifact oracle ではありません。

## MVP アーキテクチャ

```text
versioned eval manifest + policy
              │
              ▼
frozen target → trusted-local runner → deterministic checks
              │
              ▼
content-light evidence → hard-gate reducer → JSON + Markdown report
```

Manifest は相対 target root、versioned policy、argv-only command、timeout、決定論的な file check を宣言します。

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

サポートされる check type は `file_exists`、`file_equals`、`path_absent` です。不明な field、重複 YAML key、path traversal、policy drift、malformed contract は fail closed になります。

## 状態と exit code

| 状態 | Exit | 意味 |
| --- | ---: | --- |
| `PASS` | 0 | 必須の決定論的 gate がすべて成功 |
| `FAIL` | 1 | verifier、command、no-write、workspace 境界 gate が失敗 |
| `BLOCKED` | 2 | contract、policy、target、実行前提が利用できない |
| `INCONCLUSIVE` | 3 | 実行が不完全、または必要な証拠が不足 |

Hard failure は平均で相殺できません。この決定論的 MVP では model が `failure_honesty` を推測せず、`N/A` のままにします。

## Evidence contract

評価は正確に3つの成果物を作成します。

- `evidence.json`: target/policy digest、process receipt、content digest、check receipt
- `report.json`: canonical state、reason code、criteria、semantic digest
- `report.md`: `report.json` の決定論的 projection

Raw stdout、stderr、environment value、target content、credential、absolute target path は report にコピーしません。content digest と byte size だけで表現します。semantic document から timestamp と temporary path を除くため、繰り返し実行を比較できます。

## セキュリティと platform 境界

`trusted-local` は target が owner-controlled code であることを意味します。対応する hosted runtime では、Heimdall は temporary copy、argv-only subprocess、reduced environment、process-group timeout cleanup、source no-write verification を使います。

**OS sandbox ではありません:** network access と任意の host write は防止されません。adversarial code や third-party code をこの runner で評価しないでください。そのような target には将来の container または host-enforced sandbox backend が必要です。最終 acceptance とすべての外部状態遷移は人または host が管理します。

Cross-build の成功は hosted runtime の証拠とは別です。付属の Hermes Skill は配布可能ですが、自動インストールされません。

## Real-target validation

最初の trusted-local pilot では、Heimdall `27c0aa105d7216bdc8d67ee3f544e3459422d7d0` で `ddalggak` `89868c05ca781365701362db08666bca503901b2` を評価しました。2回の clean positive evaluation で同じ evidence digest `d172d008de8152b7219c3f3b661219f1c3d265015936cc34ae5b069907cd1c98` と report digest `969e6a8e589bec08aee40ffdf0ee71b67677e37796026fb3092a702f8618c15b` が得られ、target no-write は true でした。fixture-tamper probe は exit `1`、`FAIL`、`command_failed`、`required_evidence_missing` になりました。

Hosted Linux real-target lane は正確な Heimdall revision と固定された `ddalggak` revision を使い、manifest 検証、2回の評価、`PASS`、receipt 成功、semantic digest の反復、target 不変、`outside_workspace_write=false` を要求します。この限定された receipt は任意の host write を観測せず、`trusted-local` を OS sandbox に変えるものでもありません。再現コマンド：

```bash
bash scripts/verify-ddalggak-real-target.sh \
  /path/to/ddalggak /tmp/heimdall-ddalggak-real-target \
  89868c05ca781365701362db08666bca503901b2 "$(git rev-parse HEAD)"
```

## 検証と regression corpus

固定 corpus は、証拠付き PASS、証拠のない PASS-looking self-report、missing receipt、disposable target 外への write、data として扱う injection-like text を含みます。Go tests は source mutation、non-zero exit、timeout descendant cleanup、schema closure、output containment、duplicate key、symlink rejection、credential redaction も検証します。

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
internal/            contract、runner、reducer、report、決定論的 utility
schemas/             strict v1 input/evidence/report contract
policies/            versioned policy artifact
fixtures/            固定 adversarial/benign corpus
testdata/oracle/     凍結された cross-language parity receipt
skill/heimdall/      配布用 Hermes Skill；profile に自動インストールされない
templates/           embedded onboarding scaffold template
```

## License と distribution 境界

Heimdall source code は [MIT License](LICENSE) です。immutable `v0.1.0` binary release は別の checksum 検証済み distribution artifact です。source reuse、binary redistribution、stable installation channel の主張を混同しないでください。

## 今後の作業

検証済み immutable release contract を利用する reusable GitHub Action は後続作業です。Container isolation、automatic approval、deployment、numeric scoring はこの MVP の範囲外です。
