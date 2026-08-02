package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoadSpecStrictContract(t *testing.T) {
	spec, err := LoadSpec(filepath.Join(repoRoot(t), "fixtures", "pass", "eval.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if spec.TargetID != "pass" || spec.Isolation != "trusted-local" || spec.Policy["id"] != "harness-readiness" {
		t.Fatalf("unexpected spec: %#v", spec)
	}
}

func TestManifestSymlinkUsesResolvedManifestDirectory(t *testing.T) {
	original := filepath.Join(repoRoot(t), "fixtures", "pass", "eval.yaml")
	alias := filepath.Join(t.TempDir(), "eval.yaml")
	if err := os.Symlink(original, alias); err != nil {
		t.Fatal(err)
	}
	spec, err := LoadSpec(alias)
	if err != nil {
		t.Fatal(err)
	}
	if spec.ManifestPath != original {
		t.Fatalf("manifest path=%q want=%q", spec.ManifestPath, original)
	}
}

func TestDuplicateAndUnknownKeysFailClosed(t *testing.T) {
	cases := []struct{ name, body, code string }{
		{"duplicate", "schema_version: '1.0'\nschema_version: '1.0'\n", "duplicate_key"},
		{"unknown", "schema_version: '1.0'\nunexpected: true\n", "invalid_manifest"},
		{"null", "null\n", "invalid_manifest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "eval.yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadSpec(path)
			if Code(err) != tc.code {
				t.Fatalf("got %q from %v, want %q", Code(err), err, tc.code)
			}
			if err != nil && strings.Contains(err.Error(), dir) {
				t.Fatalf("error leaked path: %v", err)
			}
		})
	}
}
