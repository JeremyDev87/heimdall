package report

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteArtifactsRootDoesNotFollowReplacedOutputPath(t *testing.T) {
	parent := t.TempDir()
	outside := t.TempDir()
	out := filepath.Join(parent, "artifacts")
	original := filepath.Join(parent, "artifacts-original")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(out)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := os.Rename(out, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, out); err != nil {
		t.Fatal(err)
	}

	if err := WriteArtifactsRoot(root, map[string]any{"evidence": "ok"}, map[string]any{"state": "PASS"}, "# report\n"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"evidence.json", "report.json", "report.md"} {
		info, err := os.Stat(filepath.Join(original, name))
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("artifact %s was not written beneath the opened directory: info=%v err=%v", name, info, err)
		}
		if _, err := os.Lstat(filepath.Join(outside, name)); !os.IsNotExist(err) {
			t.Fatalf("artifact %s followed the replaced output path: %v", name, err)
		}
	}
}
