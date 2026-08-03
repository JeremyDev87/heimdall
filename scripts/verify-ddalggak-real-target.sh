#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'usage: %s <ddalggak-dir> <output-dir> <expected-ddalggak-rev> <expected-heimdall-rev>\n' "$0" >&2
}

if [ "$#" -ne 4 ]; then
  usage
  exit 64
fi

target_dir=$1
output_dir=$2
expected_target_rev=$3
expected_heimdall_rev=$4
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
heimdall_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd -P)
target_dir=$(CDPATH='' cd -- "$target_dir" && pwd -P)

export GIT_OPTIONAL_LOCKS=0
actual_heimdall_rev=$(git -C "$heimdall_dir" rev-parse HEAD)
actual_target_rev=$(git -C "$target_dir" rev-parse HEAD)

if [ "$actual_heimdall_rev" != "$expected_heimdall_rev" ]; then
  printf 'heimdall revision mismatch: expected %s, got %s\n' "$expected_heimdall_rev" "$actual_heimdall_rev" >&2
  exit 65
fi
if [ "$actual_target_rev" != "$expected_target_rev" ]; then
  printf 'ddalggak revision mismatch: expected %s, got %s\n' "$expected_target_rev" "$actual_target_rev" >&2
  exit 65
fi

heimdall_status_before=$(git -C "$heimdall_dir" status --porcelain=v1 --untracked-files=all)
target_status_before=$(git -C "$target_dir" status --porcelain=v1 --untracked-files=all)
if [ -n "$target_status_before" ]; then
  printf 'real-target verification requires a clean ddalggak worktree\n' >&2
  exit 65
fi
strict_clean=${REQUIRE_CLEAN_HEIMDALL:-0}
if [ "$strict_clean" != "0" ] && [ "$strict_clean" != "1" ]; then
  printf 'REQUIRE_CLEAN_HEIMDALL must be 0 or 1\n' >&2
  exit 64
fi
if [ "$strict_clean" = "1" ] && [ -n "$heimdall_status_before" ]; then
  printf 'strict hosted verification requires a clean Heimdall worktree\n' >&2
  exit 65
fi

case "$output_dir" in
  /*) ;;
  *)
    printf 'output directory must be absolute: %s\n' "$output_dir" >&2
    exit 64
    ;;
esac
output_parent=$(dirname -- "$output_dir")
if [ ! -d "$output_parent" ]; then
  printf 'output parent does not exist: %s\n' "$output_parent" >&2
  exit 66
fi
output_parent=$(CDPATH='' cd -- "$output_parent" && pwd -P)
output_dir="$output_parent/$(basename -- "$output_dir")"
if [[ "$output_dir" == "$heimdall_dir" || "$output_dir" == "$heimdall_dir/"* ||
      "$heimdall_dir" == "$output_dir/"* || "$output_dir" == "$target_dir" ||
      "$output_dir" == "$target_dir/"* || "$target_dir" == "$output_dir/"* ]]; then
  printf 'output directory must be outside both repository trees: %s\n' "$output_dir" >&2
  exit 64
fi
if [ -e "$output_dir" ]; then
  printf 'output directory already exists: %s\n' "$output_dir" >&2
  exit 73
fi
mkdir -p "$output_dir/run-1" "$output_dir/run-2"
mkdir -p "$output_dir/.runtime"
heimdall_bin="$output_dir/.runtime/heimdall"
(cd "$heimdall_dir" && go build -trimpath -o "$heimdall_bin" ./cmd/heimdall)
manifest="$target_dir/heimdall.eval.yaml"
"$heimdall_bin" validate "$manifest"
"$heimdall_bin" evaluate "$manifest" --out "$output_dir/run-1"
"$heimdall_bin" evaluate "$manifest" --out "$output_dir/run-2"

heimdall_status_after=$(git -C "$heimdall_dir" status --porcelain=v1 --untracked-files=all)
target_status_after=$(git -C "$target_dir" status --porcelain=v1 --untracked-files=all)
if [ "$heimdall_status_after" != "$heimdall_status_before" ]; then
  printf 'Heimdall worktree changed during real-target verification\n' >&2
  exit 1
fi
if [ "$target_status_after" != "$target_status_before" ]; then
  printf 'ddalggak worktree changed during real-target verification\n' >&2
  exit 1
fi
if [ "$(git -C "$heimdall_dir" rev-parse HEAD)" != "$actual_heimdall_rev" ] || [ "$(git -C "$target_dir" rev-parse HEAD)" != "$actual_target_rev" ]; then
  printf 'repository revision changed during real-target verification\n' >&2
  exit 1
fi

go_version=$(go version)
node - \
  "$output_dir/run-1/report.json" \
  "$output_dir/run-1/evidence.json" \
  "$output_dir/run-2/report.json" \
  "$output_dir/run-2/evidence.json" \
  "$output_dir/receipt.json" \
  "$actual_heimdall_rev" \
  "$actual_target_rev" \
  "$go_version" \
  "$heimdall_bin" \
  "$([ -z "$heimdall_status_before" ] && printf true || printf false)" \
  "$strict_clean" <<'NODE'
const fs = require("node:fs");
const os = require("node:os");
const crypto = require("node:crypto");

const [
  report1Path,
  evidence1Path,
  report2Path,
  evidence2Path,
  receiptPath,
  heimdallRevision,
  targetRevision,
  goVersion,
  heimdallBinary,
  heimdallWorktreeClean,
  strictClean,
] = process.argv.slice(2);

const readJson = (file) => JSON.parse(fs.readFileSync(file, "utf8"));
const report1 = readJson(report1Path);
const evidence1 = readJson(evidence1Path);
const report2 = readJson(report2Path);
const evidence2 = readJson(evidence2Path);
const fail = (message) => {
  throw new Error(message);
};

for (const [index, report, evidence] of [
  [1, report1, evidence1],
  [2, report2, evidence2],
]) {
  if (report.state !== "PASS") fail(`run ${index}: state is not PASS`);
  if (!Array.isArray(report.reason_codes) ||
      report.reason_codes.length !== 1 ||
      report.reason_codes[0] !== "checks_passed") {
    fail(`run ${index}: unexpected reason codes`);
  }
  if (evidence.execution?.exit_code !== 0) fail(`run ${index}: command exit is not 0`);
  if (evidence.execution?.timed_out !== false) fail(`run ${index}: command timed out`);
  if (evidence.execution?.launch_error !== false) fail(`run ${index}: command launch failed`);
  if (evidence.target?.id !== "ddalggak") fail(`run ${index}: wrong target id`);
  if (evidence.target?.no_write !== true) fail(`run ${index}: target no-write is false`);
  if (evidence.target?.digest_before !== evidence.target?.digest_after) {
    fail(`run ${index}: target digest changed`);
  }
  if (evidence.boundary?.outside_workspace_write !== false) {
    fail(`run ${index}: outside-workspace write detected`);
  }
  if (!Array.isArray(evidence.checks) || evidence.checks.length === 0) {
    fail(`run ${index}: no deterministic checks were recorded`);
  }
  if (!evidence.checks.every((check) => check.status === "PASS")) {
    fail(`run ${index}: deterministic check failed`);
  }
}

if (report1.semantic_digest !== report2.semantic_digest) fail("report semantic digests differ");
if (evidence1.semantic_digest !== evidence2.semantic_digest) fail("evidence semantic digests differ");
if (evidence1.target.digest_before !== evidence2.target.digest_before) fail("target digests differ");
if (evidence1.policy.digest !== evidence2.policy.digest) fail("policy digests differ");

const receipt = {
  schema_version: "1.0",
  platform: { os: process.platform, arch: process.arch, runner: os.release() },
  runtime: { node: process.version, go: goVersion },
  heimdall_revision: heimdallRevision,
  heimdall_source: {
    worktree_clean: heimdallWorktreeClean === "true",
    strict_clean: strictClean === "1",
    binary_sha256: crypto.createHash("sha256").update(fs.readFileSync(heimdallBinary)).digest("hex"),
  },
  target: { repository: "JeremyDev87/ddalggak", revision: targetRevision },
  outcome: {
    state: report1.state,
    no_write: evidence1.target.no_write,
    outside_workspace_write: evidence1.boundary.outside_workspace_write,
    target_digest: evidence1.target.digest_before,
    policy_digest: evidence1.policy.digest,
    evidence_semantic_digest: evidence1.semantic_digest,
    report_semantic_digest: report1.semantic_digest,
    repeat_semantic_match: true,
  },
};
fs.writeFileSync(receiptPath, `${JSON.stringify(receipt, null, 2)}\n`, { mode: 0o600 });
console.log(`REAL_TARGET_PASS evidence=${evidence1.semantic_digest} report=${report1.semantic_digest}`);
NODE
