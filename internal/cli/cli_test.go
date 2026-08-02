package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/JeremyDev87/heimdall/internal/snapshot"
)

func TestEvaluateWritesArtifactsAndExitCodes(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	for name, expected := range map[string]int{"pass": 0, "forbidden-write": 1, "false-pass": 3} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			out := filepath.Join(t.TempDir(), "out")
			code := Run([]string{"evaluate", filepath.Join(root, "fixtures", name, "eval.yaml"), "--out", out}, &stdout, &stderr)
			if code != expected || stderr.Len() != 0 {
				t.Fatalf("code=%d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["state"] == nil {
				t.Fatal("missing state")
			}
			entries, err := os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			names := []string{}
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			sort.Strings(names)
			if strings.Join(names, ",") != "evidence.json,report.json,report.md" {
				t.Fatalf("unexpected artifacts: %v", names)
			}
		})
	}
}

func TestInvalidManifestAndOutputFailClosed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	broken := filepath.Join(t.TempDir(), "broken.yaml")
	_ = os.WriteFile(broken, []byte("not: [valid"), 0o600)
	if code := Run([]string{"validate", broken}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d", code)
	}
	if stdout.String() != "{\"reason\":\"invalid_manifest\",\"state\":\"BLOCKED\"}\n" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if strings.Contains(stdout.String(), filepath.Dir(broken)) {
		t.Fatal("path leaked")
	}
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	target := filepath.Join(root, "fixtures", "pass", "target")
	out := filepath.Join(target, "reports")
	defer os.RemoveAll(out)
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"evaluate", filepath.Join(root, "fixtures", "pass", "eval.yaml"), "--out", out}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d", code)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("output was created: %v", err)
	}
}

func TestSymlinkedOutputIntoSourceFailsBeforeWriting(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	parent := t.TempDir()
	fixture := filepath.Join(parent, "fixture")
	if err := snapshot.CopyTarget(filepath.Join(root, "fixtures", "pass"), fixture); err != nil {
		t.Fatal(err)
	}
	policyDir := filepath.Join(parent, "policies")
	if err := os.Mkdir(policyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	policy, err := os.ReadFile(filepath.Join(root, "policies", "harness-readiness-v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(policyDir, "harness-readiness-v1.yaml"), policy, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(fixture, "eval.yaml")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(strings.Replace(string(data), "../../policies/", "../policies/", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(fixture, "target")
	alias := filepath.Join(parent, "output-alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"evaluate", manifest, "--out", alias}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, name := range []string{"evidence.json", "report.json", "report.md"} {
		if _, err := os.Stat(filepath.Join(target, name)); !os.IsNotExist(err) {
			t.Fatalf("artifact %s was written through symlink: %v", name, err)
		}
	}
}

func TestHelpAndUsageBoundaries(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		code   int
		stdout bool
	}{{[]string{"--help"}, 0, true}, {[]string{"validate", "--help"}, 0, true}, {[]string{"evaluate", "--help"}, 0, true}, {nil, 2, false}, {[]string{"unknown"}, 2, false}} {
		var out, err bytes.Buffer
		code := Run(tc.args, &out, &err)
		if code != tc.code {
			t.Fatalf("%v code=%d", tc.args, code)
		}
		surface := err.String()
		if tc.stdout {
			surface = out.String()
		}
		if !strings.Contains(surface, "usage: heimdall") {
			t.Fatalf("%v surface=%q", tc.args, surface)
		}
	}
}
