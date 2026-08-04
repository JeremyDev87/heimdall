#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf 'usage: %s <dist-dir> [expected-version] [expected-commit] [full|structural]\n' "$0" >&2
}

if [ "$#" -lt 1 ] || [ "$#" -gt 4 ]; then
  usage
  exit 64
fi

dist_dir=$1
expected_version=${2:-}
expected_commit=${3:-}
verification_mode=${4:-full}
case "$verification_mode" in
  full|structural) ;;
  *)
    usage
    exit 64
    ;;
esac
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
repo_dir=$(CDPATH='' cd -- "$script_dir/.." && pwd -P)

if [ ! -d "$dist_dir" ]; then
  printf 'distribution directory does not exist: %s\n' "$dist_dir" >&2
  exit 66
fi
dist_dir=$(CDPATH='' cd -- "$dist_dir" && pwd -P)
if [ ! -f "$dist_dir/checksums.txt" ] || [ -L "$dist_dir/checksums.txt" ]; then
  printf 'missing or unsafe checksums.txt\n' >&2
  exit 66
fi

shopt -s nullglob
archives=("$dist_dir"/*.tar.gz)
linux_archives=("$dist_dir"/heimdall_*_linux_amd64.tar.gz)
darwin_archives=("$dist_dir"/heimdall_*_darwin_arm64.tar.gz)
if [ "${#archives[@]}" -ne 2 ] || [ "${#linux_archives[@]}" -ne 1 ] || [ "${#darwin_archives[@]}" -ne 1 ]; then
  printf 'expected exactly linux_amd64 and darwin_arm64 archives; found %d total, %d linux, %d darwin\n' \
    "${#archives[@]}" "${#linux_archives[@]}" "${#darwin_archives[@]}" >&2
  exit 65
fi
for archive in "${archives[@]}"; do
  if [ ! -f "$archive" ] || [ -L "$archive" ]; then
    printf 'archive must be a regular non-symlink file: %s\n' "$archive" >&2
    exit 65
  fi
done

linux_name=$(basename -- "${linux_archives[0]}")
version=${linux_name#heimdall_}
version=${version%_linux_amd64.tar.gz}
if [ -z "$version" ]; then
  printf 'could not derive version from %s\n' "$linux_name" >&2
  exit 65
fi
darwin_name="heimdall_${version}_darwin_arm64.tar.gz"
if [ "$(basename -- "${darwin_archives[0]}")" != "$darwin_name" ]; then
  printf 'archive versions disagree: %s vs %s\n' "$linux_name" "$(basename -- "${darwin_archives[0]}")" >&2
  exit 65
fi
if [ -n "$expected_version" ] && [ "$version" != "${expected_version#v}" ]; then
  printf 'version mismatch: expected %s, got %s\n' "${expected_version#v}" "$version" >&2
  exit 65
fi

actual_checksum_names=$(awk 'NF == 2 { print $2 }' "$dist_dir/checksums.txt" | LC_ALL=C sort)
expected_checksum_names=$(printf '%s\n%s\n' "$darwin_name" "$linux_name" | LC_ALL=C sort)
if [ "$actual_checksum_names" != "$expected_checksum_names" ]; then
  printf 'checksum manifest must name exactly the two release archives\n' >&2
  printf 'expected:\n%s\nactual:\n%s\n' "$expected_checksum_names" "$actual_checksum_names" >&2
  exit 65
fi
if grep -Eq '(^|[[:space:]])(/|\.\./)' "$dist_dir/checksums.txt"; then
  printf 'checksum manifest contains an unsafe path\n' >&2
  exit 65
fi

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$dist_dir" && sha256sum --check checksums.txt)
else
  (cd "$dist_dir" && shasum -a 256 --check checksums.txt)
fi

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/heimdall-release-verify.XXXXXX")
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

repo_status_before=$(git -C "$repo_dir" status --porcelain=v1 --untracked-files=all)
verify_archive() {
  local os_name=$1
  local arch_name=$2
  local archive_path=$3
  local extract_dir="$tmp_dir/${os_name}_${arch_name}"
  local actual_members
  local expected_members
  local module_info

  actual_members=$(tar -tzf "$archive_path" | sed 's#^\./##' | LC_ALL=C sort)
  expected_members=$(printf 'LICENSE\nREADME.md\nheimdall\n' | LC_ALL=C sort)
  if [ "$actual_members" != "$expected_members" ]; then
    printf 'unexpected members in %s\nexpected:\n%s\nactual:\n%s\n' \
      "$(basename -- "$archive_path")" "$expected_members" "$actual_members" >&2
    exit 65
  fi
  python3 - "$archive_path" <<'PY'
import sys
import tarfile

expected = {"LICENSE", "README.md", "heimdall"}
names = []
with tarfile.open(sys.argv[1], mode="r:gz") as archive:
    for member in archive.getmembers():
        name = member.name[2:] if member.name.startswith("./") else member.name
        names.append(name)
        if name not in expected or not member.isfile():
            raise SystemExit(f"unsafe archive member: {member.name!r} type={member.type!r}")
        if name == "heimdall" and member.mode & 0o111 == 0:
            raise SystemExit("packaged heimdall is not executable")
if len(names) != len(expected) or set(names) != expected:
    raise SystemExit(f"unexpected or duplicate archive members: {names!r}")
PY
  if [ "$verification_mode" = structural ]; then
    return
  fi

  mkdir -p "$extract_dir"
  tar -xzf "$archive_path" -C "$extract_dir"
  if [ ! -x "$extract_dir/heimdall" ]; then
    printf 'packaged binary is not executable: %s\n' "$archive_path" >&2
    exit 65
  fi
  module_info=$(go version -m "$extract_dir/heimdall")
  printf '%s\n' "$module_info" | grep -F "GOOS=$os_name" >/dev/null
  printf '%s\n' "$module_info" | grep -F "GOARCH=$arch_name" >/dev/null
}

verify_archive linux amd64 "${linux_archives[0]}"
verify_archive darwin arm64 "${darwin_archives[0]}"

if [ "$verification_mode" = structural ]; then
  printf 'release structure verified without executing packaged binaries: version=%s\n' "$version"
  exit 0
fi

host_os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
  x86_64) host_arch=amd64 ;;
  arm64|aarch64) host_arch=arm64 ;;
  *) host_arch=unsupported ;;
esac

case "$host_os/$host_arch" in
  linux/amd64|darwin/arm64) ;;
  *)
    printf 'release structure verified; native runtime smoke skipped on %s/%s\n' "$host_os" "$host_arch"
    exit 0
    ;;
esac

binary="$tmp_dir/${host_os}_${host_arch}/heimdall"
version_json=$("$binary" version)
python3 - "$version_json" "$version" "$expected_commit" <<'PY'
import json
import sys

payload = json.loads(sys.argv[1])
expected_version = sys.argv[2]
expected_commit = sys.argv[3]
if payload.get("version") != expected_version:
    raise SystemExit(f"embedded version mismatch: {payload.get('version')!r} != {expected_version!r}")
if expected_commit and payload.get("commit") != expected_commit:
    raise SystemExit(f"embedded commit mismatch: {payload.get('commit')!r} != {expected_commit!r}")
if payload.get("commit") in (None, "", "unknown"):
    raise SystemExit("embedded commit is missing")
if payload.get("build_date") in (None, "", "unknown"):
    raise SystemExit("embedded build_date is missing")
PY
printf 'verified packaged heimdall version for %s/%s\n' "$host_os" "$host_arch"
"$binary" --help | grep -F 'Evaluate a trusted local agent harness' >/dev/null

pass_1="$tmp_dir/pass-1"
pass_2="$tmp_dir/pass-2"
"$binary" evaluate "$repo_dir/fixtures/pass/eval.yaml" --out "$pass_1" > "$tmp_dir/pass-1.stdout"
"$binary" evaluate "$repo_dir/fixtures/pass/eval.yaml" --out "$pass_2" > "$tmp_dir/pass-2.stdout"

set +e
"$binary" evaluate "$repo_dir/fixtures/false-pass/eval.yaml" --out "$tmp_dir/false-pass" > "$tmp_dir/false-pass.stdout"
false_pass_exit=$?
set -e
if [ "$false_pass_exit" -ne 3 ]; then
  printf 'false-pass exit=%s, want 3\n' "$false_pass_exit" >&2
  exit 65
fi

python3 - \
  "$pass_1/report.json" "$pass_2/report.json" "$tmp_dir/false-pass/report.json" <<'PY'
import json
import sys

pass_1, pass_2, false_pass = [json.load(open(path, encoding="utf-8")) for path in sys.argv[1:]]
if pass_1.get("state") != "PASS" or pass_2.get("state") != "PASS":
    raise SystemExit("packaged PASS fixture did not pass twice")
if pass_1.get("semantic_digest") != pass_2.get("semantic_digest"):
    raise SystemExit("packaged PASS semantic digest changed across repeat runs")
if false_pass.get("state") != "INCONCLUSIVE":
    raise SystemExit(f"false-pass state={false_pass.get('state')!r}, want INCONCLUSIVE")
PY

repo_status_after=$(git -C "$repo_dir" status --porcelain=v1 --untracked-files=all)
if [ "$repo_status_after" != "$repo_status_before" ]; then
  printf 'packaged runtime smoke changed the source worktree\n' >&2
  exit 65
fi
printf 'release verification passed: version=%s tuple=%s/%s\n' "$version" "$host_os" "$host_arch"
