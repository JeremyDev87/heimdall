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

func TestWriteArtifactsRootDoesNotPartiallyPublishWhenSecondArtifactFails(t *testing.T) {
	out := t.TempDir()
	root, err := os.OpenRoot(out)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	oldEvidence := map[string]any{"generation": "old", "artifact": "evidence"}
	oldReport := map[string]any{"generation": "old", "artifact": "report"}
	if err := WriteArtifactsRoot(root, oldEvidence, oldReport, "old markdown\n"); err != nil {
		t.Fatal(err)
	}
	oldEvidenceBytes, err := os.ReadFile(filepath.Join(out, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldMarkdown, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(out, "report.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(out, "report.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	err = WriteArtifactsRoot(root, map[string]any{"generation": "new"}, map[string]any{"generation": "new"}, "new markdown\n")
	if err == nil {
		t.Fatal("expected second-artifact publication failure")
	}
	assertArtifactBytes(t, filepath.Join(out, "evidence.json"), oldEvidenceBytes)
	assertArtifactBytes(t, filepath.Join(out, "report.md"), oldMarkdown)
	assertDirectory(t, filepath.Join(out, "report.json"))
}

func TestWriteArtifactsRootDoesNotPartiallyPublishWhenThirdArtifactFails(t *testing.T) {
	out := t.TempDir()
	root, err := os.OpenRoot(out)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	oldEvidence := map[string]any{"generation": "old", "artifact": "evidence"}
	oldReport := map[string]any{"generation": "old", "artifact": "report"}
	if err := WriteArtifactsRoot(root, oldEvidence, oldReport, "old markdown\n"); err != nil {
		t.Fatal(err)
	}
	oldEvidenceBytes, err := os.ReadFile(filepath.Join(out, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldReportBytes, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(out, "report.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(out, "report.md"), 0o700); err != nil {
		t.Fatal(err)
	}

	err = WriteArtifactsRoot(root, map[string]any{"generation": "new"}, map[string]any{"generation": "new"}, "new markdown\n")
	if err == nil {
		t.Fatal("expected third-artifact publication failure")
	}
	assertArtifactBytes(t, filepath.Join(out, "evidence.json"), oldEvidenceBytes)
	assertArtifactBytes(t, filepath.Join(out, "report.json"), oldReportBytes)
	assertDirectory(t, filepath.Join(out, "report.md"))
}

func TestRollbackPublishedArtifactsRestoresEveryOriginalArtifact(t *testing.T) {
	out := t.TempDir()
	root, err := os.OpenRoot(out)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	artifacts := []artifactSpec{
		{name: "evidence.json"},
		{name: "report.json"},
		{name: "report.md"},
	}
	backups := make([]artifactBackup, 0, len(artifacts))
	for _, artifact := range artifacts {
		backup := "." + artifact.name + ".bak-fixture"
		if err := os.WriteFile(filepath.Join(out, backup), []byte("old "+artifact.name), 0o600); err != nil {
			t.Fatal(err)
		}
		backups = append(backups, artifactBackup{name: artifact.name, backup: backup, hadOriginal: true})
	}
	if err := os.WriteFile(filepath.Join(out, "evidence.json"), []byte("new evidence"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := rollbackPublishedArtifacts(root, artifacts, backups); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		assertArtifactBytes(t, filepath.Join(out, artifact.name), []byte("old "+artifact.name))
	}
}

func assertArtifactBytes(t *testing.T, path string, expected []byte) {
	t.Helper()
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(actual) != string(expected) {
		t.Fatalf("artifact %s changed after failed publication: got %q want %q", path, actual, expected)
	}
}

func assertDirectory(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
}
