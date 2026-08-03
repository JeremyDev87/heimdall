package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/JeremyDev87/heimdall/internal/contract"
	"github.com/JeremyDev87/heimdall/internal/snapshot"
)

func TestInitCreatesReviewableScaffoldAndCheckPreservesSource(t *testing.T) {
	target := t.TempDir()
	command := filepath.Join(target, "verify-input.sh")
	writeExecutable(t, command, `#!/bin/sh
set -eu
[ "$1" = "two words" ]
[ "$2" = '$HOME' ]
[ "$3" = "quote'arg" ]
printf 'ran\n' > command-ran.txt
`)

	before, err := snapshot.TreeDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"init", "--preset", "command-artifact", "--target", target, "--", "./verify-input.sh", "two words", "$HOME", "quote'arg"}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	payload := decodeLine(t, stdout.Bytes())
	if payload["state"] != "PASS" || payload["reason"] != "scaffold_created" || payload["preset"] != "command-artifact" {
		t.Fatalf("payload=%v", payload)
	}
	if strings.Contains(stdout.String(), target) {
		t.Fatal("init output leaked the absolute target path")
	}

	scaffold := filepath.Join(target, ".heimdall")
	entries, err := os.ReadDir(scaffold)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "verify-harness.sh" {
		t.Fatalf("unexpected scaffold: %v", names)
	}
	wrapperInfo, err := os.Stat(filepath.Join(scaffold, "verify-harness.sh"))
	if err != nil || wrapperInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("wrapper is not executable: info=%v err=%v", wrapperInfo, err)
	}
	manifest := filepath.Join(target, ".heimdall-eval.yaml")
	if _, err := contract.LoadSpec(manifest); err != nil {
		t.Fatalf("generated manifest is invalid: %v", err)
	}
	afterInit, err := snapshot.TreeDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	if before == afterInit {
		t.Fatal("init did not create the requested scaffold")
	}

	out := filepath.Join(t.TempDir(), "artifacts")
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"check", manifest, "--out", out}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("check code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	payload = decodeLine(t, stdout.Bytes())
	resolvedOut, err := resolveForContainment(out)
	if err != nil {
		t.Fatal(err)
	}
	if payload["state"] != "PASS" || payload["artifacts_dir"] != resolvedOut {
		t.Fatalf("check payload=%v", payload)
	}
	for _, name := range []string{"evidence.json", "report.json", "report.md"} {
		if info, err := os.Stat(filepath.Join(out, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("missing artifact %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "command-ran.txt")); !os.IsNotExist(err) {
		t.Fatalf("target command escaped disposable copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(scaffold, "heimdall-ok")); !os.IsNotExist(err) {
		t.Fatalf("freshness marker escaped disposable copy: %v", err)
	}
	afterCheck, err := snapshot.TreeDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	if afterInit != afterCheck {
		t.Fatalf("check modified source: before=%s after=%s", afterInit, afterCheck)
	}
}

func TestInitIsIdempotentAndFailsClosedOnConflict(t *testing.T) {
	target := t.TempDir()
	command := filepath.Join(target, "verify.sh")
	writeExecutable(t, command, "#!/bin/sh\nexit 0\n")
	args := []string{"init", "--target", target, "--", "./verify.sh"}
	var stdout, stderr bytes.Buffer
	if code := Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("first init code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("repeat init code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if payload := decodeLine(t, stdout.Bytes()); payload["reason"] != "scaffold_unchanged" {
		t.Fatalf("payload=%v", payload)
	}

	manifest := filepath.Join(target, ".heimdall-eval.yaml")
	changed := []byte("owner content\n")
	if err := os.WriteFile(manifest, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(args, &stdout, &stderr); code != 2 {
		t.Fatalf("conflict code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if payload := decodeLine(t, stdout.Bytes()); payload["reason"] != "scaffold_conflict" || payload["state"] != "BLOCKED" {
		t.Fatalf("payload=%v", payload)
	}
	data, err := os.ReadFile(manifest)
	if err != nil || !bytes.Equal(data, changed) {
		t.Fatalf("conflicting file was overwritten: data=%q err=%v", data, err)
	}
}

func TestInitRejectsSymlinkedScaffoldAndInvalidArgumentsBeforeWriting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	target := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(target, ".heimdall")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", "--target", target, "--", "./verify.sh"}, &stdout, &stderr); code != 2 {
		t.Fatalf("symlink code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("symlink destination changed: entries=%v err=%v", entries, err)
	}

	for _, args := range [][]string{
		{"init", "--preset", "unknown", "--target", t.TempDir(), "--", "./verify.sh"},
		{"init", "--target", t.TempDir(), "--"},
		{"init", "--target", t.TempDir(), "--", ""},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := Run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestCheckDefaultsAndPreservesEvaluateCompatibility(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	blockedRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(blockedRoot, "target"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy, err := os.ReadFile(filepath.Join(root, "policies", "harness-readiness-v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blockedRoot, "policy.yaml"), policy, 0o600); err != nil {
		t.Fatal(err)
	}
	blockedManifest := filepath.Join(blockedRoot, "eval.yaml")
	blockedSpec := "schema_version: '1.0'\ntarget:\n  id: blocked\n  root: target\npolicy:\n  id: harness-readiness\n  version: '1'\n  path: policy.yaml\nisolation: trusted-local\ncommand:\n  argv: [/heimdall-command-does-not-exist]\n  timeout_seconds: 10\nchecks:\n- id: result\n  kind: file_exists\n  path: result.txt\n"
	if err := os.WriteFile(blockedManifest, []byte(blockedSpec), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		manifest string
		expected int
	}{
		"pass":            {filepath.Join(root, "fixtures", "pass", "eval.yaml"), 0},
		"forbidden-write": {filepath.Join(root, "fixtures", "forbidden-write", "eval.yaml"), 1},
		"false-pass":      {filepath.Join(root, "fixtures", "false-pass", "eval.yaml"), 3},
		"blocked":         {blockedManifest, 2},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			var checkOut, checkErr bytes.Buffer
			out := filepath.Join(t.TempDir(), "check")
			code := Run([]string{"check", testCase.manifest, "--out", out}, &checkOut, &checkErr)
			if code != testCase.expected || checkErr.Len() != 0 {
				t.Fatalf("check code=%d stdout=%q stderr=%q", code, checkOut.String(), checkErr.String())
			}
			checkPayload := decodeLine(t, checkOut.Bytes())
			resolvedOut, err := resolveForContainment(out)
			if err != nil {
				t.Fatal(err)
			}
			if checkPayload["artifacts_dir"] != resolvedOut {
				t.Fatalf("check payload=%v", checkPayload)
			}

			var evalOut, evalErr bytes.Buffer
			legacyOut := filepath.Join(t.TempDir(), "evaluate")
			legacyCode := Run([]string{"evaluate", testCase.manifest, "--out", legacyOut}, &evalOut, &evalErr)
			if legacyCode != testCase.expected || evalErr.Len() != 0 {
				t.Fatalf("evaluate code=%d stdout=%q stderr=%q", legacyCode, evalOut.String(), evalErr.String())
			}
			legacyPayload := decodeLine(t, evalOut.Bytes())
			if _, ok := legacyPayload["artifacts_dir"]; ok {
				t.Fatalf("legacy evaluate output changed: %v", legacyPayload)
			}
			for _, key := range []string{"state", "evidence_digest", "report_digest"} {
				if checkPayload[key] != legacyPayload[key] {
					t.Fatalf("%s mismatch: check=%v evaluate=%v", key, checkPayload, legacyPayload)
				}
			}
		})
	}
	invalidManifest := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(invalidManifest, []byte("not: [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidOut := filepath.Join(t.TempDir(), "artifacts")
	var invalidStdout, invalidStderr bytes.Buffer
	if code := Run([]string{"check", invalidManifest, "--out", invalidOut}, &invalidStdout, &invalidStderr); code != 2 {
		t.Fatalf("invalid check code=%d stdout=%q stderr=%q", code, invalidStdout.String(), invalidStderr.String())
	}
	if _, err := os.Stat(invalidOut); !os.IsNotExist(err) {
		t.Fatalf("invalid manifest created artifacts: %v", err)
	}
}

func TestCheckUsesDefaultManifestAndTemporaryOutput(t *testing.T) {
	target := t.TempDir()
	writeExecutable(t, filepath.Join(target, "verify.sh"), "#!/bin/sh\nexit 0\n")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", "--target", target, "--", "./verify.sh"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(target); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCWD) })
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("check code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	payload := decodeLine(t, stdout.Bytes())
	artifactDir, ok := payload["artifacts_dir"].(string)
	if !ok || artifactDir == "" {
		t.Fatalf("payload=%v", payload)
	}
	t.Cleanup(func() { _ = os.RemoveAll(artifactDir) })
	targetResolved, _ := filepath.EvalSymlinks(target)
	outResolved, err := resolveForContainment(artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := filepath.Rel(targetResolved, outResolved); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("default output is inside target: %s", artifactDir)
	}
}

func TestCheckRejectsOutputDirectorySwapBeforeWritingReports(t *testing.T) {
	target := t.TempDir()
	out := filepath.Join(t.TempDir(), "artifacts")
	writeExecutable(t, filepath.Join(target, "swap-output.sh"), `#!/bin/sh
set -eu
rm -rf -- "$1"
ln -s -- "$2" "$1"
`)
	var stdout, stderr bytes.Buffer
	args := []string{"init", "--target", target, "--", "./swap-output.sh", out, target}
	if code := Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("init code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	manifest := filepath.Join(target, ".heimdall-eval.yaml")
	if code := Run([]string{"check", manifest, "--out", out}, &stdout, &stderr); code != 2 {
		t.Fatalf("check code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	payload := decodeLine(t, stdout.Bytes())
	if payload["state"] != "BLOCKED" || payload["reason"] != "invalid_output" {
		t.Fatalf("payload=%v", payload)
	}
	for _, name := range []string{"evidence.json", "report.json", "report.md"} {
		if _, err := os.Stat(filepath.Join(target, name)); !os.IsNotExist(err) {
			t.Fatalf("report escaped into source: %s err=%v", name, err)
		}
	}
}

func TestCheckDoesNotFollowPredictableArtifactTempSymlink(t *testing.T) {
	target := t.TempDir()
	writeExecutable(t, filepath.Join(target, "verify.sh"), "#!/bin/sh\nexit 0\n")
	victim := filepath.Join(target, "owner-content.txt")
	if err := os.WriteFile(victim, []byte("owner content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(out, ".evidence.json.tmp")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", "--target", target, "--", "./verify.sh"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	manifest := filepath.Join(target, ".heimdall-eval.yaml")
	if code := Run([]string{"check", manifest, "--out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("check code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	content, err := os.ReadFile(victim)
	if err != nil || string(content) != "owner content\n" {
		t.Fatalf("artifact temp symlink modified source: content=%q err=%v", content, err)
	}
	if info, err := os.Lstat(filepath.Join(out, "evidence.json")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("evidence artifact is not a regular file: info=%v err=%v", info, err)
	}
}

func TestVersionAndOnboardingHelp(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := Version, Commit, BuildDate
	Version, Commit, BuildDate = "v0.1.0", "abc123", "2026-08-03T00:00:00Z"
	t.Cleanup(func() { Version, Commit, BuildDate = oldVersion, oldCommit, oldBuildDate })
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"version"}, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("version code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stdout.String() != "{\"build_date\":\"2026-08-03T00:00:00Z\",\"commit\":\"abc123\",\"version\":\"v0.1.0\"}\n" {
		t.Fatalf("version output=%q", stdout.String())
	}
	for _, args := range [][]string{{"--help"}, {"init", "--help"}, {"check", "--help"}, {"version", "--help"}} {
		stdout.Reset()
		stderr.Reset()
		if code := Run(args, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "usage: heimdall") {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestNormalizedTargetIDPreservesValidNumericPrefix(t *testing.T) {
	if got := normalizedTargetID("123 Harness"); got != "123-harness" {
		t.Fatalf("normalizedTargetID=%q", got)
	}
	if got := normalizedTargetID("한글"); got != "local-harness" {
		t.Fatalf("normalizedTargetID fallback=%q", got)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func decodeLine(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid JSON line %q: %v", data, err)
	}
	return payload
}
