#!/usr/bin/env bash
set -uo pipefail

RELEASE_VERSION='0.1.0'
RELEASE_COMMIT='1cc04368aebe25d459cc65796855a9f3e9ce3338'
LINUX_SHA='55700942cfec80c2c00f3a21c0c0ad3bb4fe5b8d09ef5f362b7b9f8a3acc2957'
DARWIN_SHA='883a564c95b58117d6803ea058ea54a11cfac19b2af17044c26306b40fc218db'

output_file=${GITHUB_OUTPUT:-}
runner_temp=${RUNNER_TEMP:-}
workspace=${GITHUB_WORKSPACE:-}
runner_os=${RUNNER_OS:-}
runner_arch=${RUNNER_ARCH:-}
manifest_rel=${INPUT_MANIFEST:-.heimdall-eval.yaml}
action_error=125

emit() {
  local state=$1 exit_code=$2 artifacts_dir=${3:-} evidence_digest=${4:-} report_digest=${5:-} binary_path=${6:-} binary_version=${7:-} binary_commit=${8:-}
  if [ -n "$output_file" ]; then
    {
      printf 'state=%s\n' "$state"
      printf 'exit-code=%s\n' "$exit_code"
      printf 'artifacts-dir=%s\n' "$artifacts_dir"
      printf 'evidence-digest=%s\n' "$evidence_digest"
      printf 'report-digest=%s\n' "$report_digest"
      printf 'binary-path=%s\n' "$binary_path"
      printf 'binary-version=%s\n' "$binary_version"
      printf 'binary-commit=%s\n' "$binary_commit"
    } >> "$output_file"
  fi
}

fail_action() {
  local message=$1
  printf 'Heimdall Action error: %s\n' "$message" >&2
  emit ACTION_ERROR "$action_error"
  exit "$action_error"
}

case "$runner_os/$runner_arch" in
  Linux/X64)
    asset="heimdall_${RELEASE_VERSION}_linux_amd64.tar.gz"
    expected_sha=$LINUX_SHA
    ;;
  macOS/ARM64)
    asset="heimdall_${RELEASE_VERSION}_darwin_arm64.tar.gz"
    expected_sha=$DARWIN_SHA
    ;;
  *)
    fail_action "unsupported runner tuple; supported tuples are Linux/X64 and macOS/ARM64"
    ;;
esac

[ -n "$runner_temp" ] && [ -d "$runner_temp" ] || fail_action 'RUNNER_TEMP is missing or is not a directory'
[ -n "$workspace" ] && [ -d "$workspace" ] || fail_action 'GITHUB_WORKSPACE is missing or is not a directory'
[ -n "$output_file" ] || fail_action 'GITHUB_OUTPUT is missing'

case "$manifest_rel" in
  ''|/*|*\\*|*:*|*$'\n'*|*$'\r'*)
    fail_action 'manifest must be a workspace-relative POSIX path'
    ;;
esac
IFS='/' read -r -a manifest_parts <<< "$manifest_rel"
for part in "${manifest_parts[@]}"; do
  [ -n "$part" ] || fail_action 'manifest contains an empty path component'
  [ "$part" != '..' ] || fail_action 'manifest must not escape the workspace'
done

workspace_real=$(cd "$workspace" && pwd -P) || fail_action 'cannot resolve GITHUB_WORKSPACE'
manifest_path="$workspace/$manifest_rel"
manifest_dir=$(dirname -- "$manifest_path")
manifest_base=$(basename -- "$manifest_path")
[ -f "$manifest_path" ] && [ ! -L "$manifest_path" ] || fail_action 'manifest is missing or is a symlink'
manifest_dir_real=$(cd "$manifest_dir" && pwd -P) || fail_action 'cannot resolve manifest directory'
manifest_real="$manifest_dir_real/$manifest_base"
case "$manifest_real" in
  "$workspace_real"/*) ;;
  *) fail_action 'manifest resolves outside GITHUB_WORKSPACE' ;;
esac

install_dir=$(mktemp -d "$runner_temp/heimdall-action.XXXXXX") || fail_action 'cannot create private install directory'
artifacts_dir=$(mktemp -d "$runner_temp/heimdall-artifacts.XXXXXX") || fail_action 'cannot create private artifacts directory'
archive="$install_dir/$asset"
binary="$install_dir/heimdall"
url="https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/$asset"

if ! curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 --retry 2 --connect-timeout 15 --max-time 120 -o "$archive" "$url"; then
  fail_action 'fixed v0.1.0 release download failed'
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual_sha=$(sha256sum "$archive" | awk '{print $1}')
else
  actual_sha=$(shasum -a 256 "$archive" | awk '{print $1}')
fi
[ "$actual_sha" = "$expected_sha" ] || fail_action 'downloaded release checksum mismatch'

members=$(tar -tzf "$archive" | sed 's#^\./##' | LC_ALL=C sort) || fail_action 'release archive inspection failed'
expected_members=$(printf 'LICENSE\nREADME.md\nheimdall\n' | LC_ALL=C sort)
[ "$members" = "$expected_members" ] || fail_action 'release archive members are not the fixed safe set'
if ! tar -xzf "$archive" -C "$install_dir"; then
  fail_action 'release archive extraction failed'
fi
[ -x "$binary" ] || fail_action 'release binary is missing or not executable'

version_json=$($binary version 2>/dev/null) || fail_action 'release binary version command failed'
printf '%s' "$version_json" | grep -F '"version":"0.1.0"' >/dev/null || fail_action 'release binary version mismatch'
printf '%s' "$version_json" | grep -F "\"commit\":\"$RELEASE_COMMIT\"" >/dev/null || fail_action 'release binary commit mismatch'
printf '%s' "$version_json" | grep -Eq '"build_date":"[^"]+"' || fail_action 'release binary build date is missing'
binary_version=$RELEASE_VERSION
binary_commit=$RELEASE_COMMIT

set +e
binary_output=$($binary check "$manifest_real" --out "$artifacts_dir" 2>"$install_dir/heimdall.stderr")
verdict=$?
set -e
printf '%s\n' "$binary_output"
if [ "$verdict" -lt 0 ] || [ "$verdict" -gt 3 ]; then
  cat "$install_dir/heimdall.stderr" >&2
  emit ACTION_ERROR "$verdict" "$artifacts_dir" '' '' "$binary" "$binary_version" "$binary_commit"
  exit "$verdict"
fi

binary_state=$(printf '%s\n' "$binary_output" | sed -n 's/.*"state":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
if [ "$verdict" -eq 2 ] && [ "$binary_state" = BLOCKED ] && [ ! -f "$artifacts_dir/report.json" ]; then
  emit BLOCKED 2 "$artifacts_dir" '' '' "$binary" "$binary_version" "$binary_commit"
  exit 2
fi

for artifact in evidence.json report.json report.md; do
  [ -f "$artifacts_dir/$artifact" ] && [ ! -L "$artifacts_dir/$artifact" ] || {
    printf 'Heimdall Action error: expected artifact is missing: %s\n' "$artifact" >&2
    emit ACTION_ERROR "$action_error" "$artifacts_dir" '' '' "$binary" "$binary_version" "$binary_commit"
    exit "$action_error"
  }
done

state=$(sed -n 's/.*"state":[[:space:]]*"\([^"]*\)".*/\1/p' "$artifacts_dir/report.json" | head -n 1)
evidence_digest=$(sed -n 's/.*"digest":[[:space:]]*"\([^"]*\)".*/\1/p' "$artifacts_dir/evidence.json" | head -n 1)
report_digest=$(sed -n 's/.*"semantic_digest":[[:space:]]*"\([^"]*\)".*/\1/p' "$artifacts_dir/report.json" | head -n 1)
case "$state" in
  PASS|FAIL|BLOCKED|INCONCLUSIVE) ;;
  *)
    printf 'Heimdall Action error: report state is invalid\n' >&2
    emit ACTION_ERROR "$action_error" "$artifacts_dir" "$evidence_digest" "$report_digest" "$binary" "$binary_version" "$binary_commit"
    exit "$action_error"
    ;;
esac

emit "$state" "$verdict" "$artifacts_dir" "$evidence_digest" "$report_digest" "$binary" "$binary_version" "$binary_commit"
exit "$verdict"
