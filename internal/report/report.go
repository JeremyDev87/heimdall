package report

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JeremyDev87/heimdall/internal/canonjson"
)

func RenderMarkdown(report map[string]any) string {
	target := report["target"].(map[string]any)
	policy := report["policy"].(map[string]any)
	evidence := report["evidence"].(map[string]any)
	reasonValues := report["reason_codes"].([]string)
	lines := []string{"# Heimdall Assessment", "", fmt.Sprintf("- State: `%s`", report["state"]), fmt.Sprintf("- Target: `%s`", target["id"]), fmt.Sprintf("- Target digest: `%s`", target["digest"]), fmt.Sprintf("- Policy: `%s@%s`", policy["id"], policy["version"]), fmt.Sprintf("- Evidence digest: `%s`", evidence["digest"]), fmt.Sprintf("- Reasons: `%s`", strings.Join(reasonValues, ", ")), "", "| Criterion | Status |", "| --- | --- |"}
	for _, raw := range report["criteria"].([]any) {
		item := raw.(map[string]any)
		lines = append(lines, fmt.Sprintf("| %s | %s |", item["id"], item["status"]))
	}
	lines = append(lines, "", "> trusted-local execution uses a temporary copy and sanitized environment; it is not a security sandbox.", "")
	return strings.Join(lines, "\n")
}

func WriteArtifacts(out string, evidence, assessment map[string]any, markdown string) error {
	directory, err := filepath.Abs(expandHome(out))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	return WriteArtifactsRoot(root, evidence, assessment, markdown)
}

// WriteArtifactsRoot writes only beneath the already-open output directory.
// The caller owns the root and is responsible for checking its containment.
func WriteArtifactsRoot(root *os.Root, evidence, assessment map[string]any, markdown string) error {
	if root == nil {
		return fmt.Errorf("nil output root")
	}
	evidenceJSON, err := canonjson.MarshalIndent(evidence)
	if err != nil {
		return err
	}
	reportJSON, err := canonjson.MarshalIndent(assessment)
	if err != nil {
		return err
	}
	return publishArtifactsRoot(root, []artifactSpec{
		{name: "evidence.json", content: append(evidenceJSON, '\n')},
		{name: "report.json", content: append(reportJSON, '\n')},
		{name: "report.md", content: []byte(markdown)},
	})
}

type artifactSpec struct {
	name    string
	content []byte
}

type artifactBackup struct {
	name        string
	backup      string
	hadOriginal bool
}

func publishArtifactsRoot(root *os.Root, artifacts []artifactSpec) error {
	temporaryNames := make([]string, 0, len(artifacts))
	cleanupTemporary := func() {
		for _, name := range temporaryNames {
			_ = root.Remove(name)
		}
	}
	defer cleanupTemporary()

	for _, artifact := range artifacts {
		temporary, handle, err := createTempRootFile(root, "."+artifact.name+".tmp-")
		if err != nil {
			return err
		}
		temporaryNames = append(temporaryNames, temporary)
		if _, err := handle.Write(artifact.content); err != nil {
			_ = handle.Close()
			return err
		}
		if err := handle.Close(); err != nil {
			return err
		}
	}

	// Validate every destination before moving any existing generation. A
	// directory, symlink, or other non-regular destination must fail closed.
	exists := make([]bool, len(artifacts))
	for i, artifact := range artifacts {
		info, err := root.Lstat(artifact.name)
		switch {
		case err == nil:
			if !info.Mode().IsRegular() {
				return fmt.Errorf("artifact destination %q is not a regular file", artifact.name)
			}
			exists[i] = true
		case os.IsNotExist(err):
		default:
			return err
		}
	}

	backups := make([]artifactBackup, 0, len(artifacts))
	reservedBackups := make([]string, 0, len(artifacts))
	cleanupReserved := func() {
		for _, name := range reservedBackups {
			if name != "" {
				_ = root.Remove(name)
			}
		}
	}
	defer cleanupReserved()

	for i, artifact := range artifacts {
		backupName, handle, err := createTempRootFile(root, "."+artifact.name+".bak-")
		if err != nil {
			return errors.Join(err, restoreArtifactBackups(root, backups))
		}
		reservedBackups = append(reservedBackups, backupName)
		if err := handle.Close(); err != nil {
			return errors.Join(err, restoreArtifactBackups(root, backups))
		}
		if !exists[i] {
			if err := root.Remove(backupName); err != nil {
				return errors.Join(err, restoreArtifactBackups(root, backups))
			}
			reservedBackups[len(reservedBackups)-1] = ""
			backups = append(backups, artifactBackup{name: artifact.name})
			continue
		}
		if err := root.Rename(artifact.name, backupName); err != nil {
			return errors.Join(err, restoreArtifactBackups(root, backups))
		}
		backups = append(backups, artifactBackup{name: artifact.name, backup: backupName, hadOriginal: true})
	}

	for i, artifact := range artifacts {
		if err := root.Rename(temporaryNames[i], artifact.name); err != nil {
			rollbackErr := rollbackPublishedArtifacts(root, artifacts, backups)
			return errors.Join(fmt.Errorf("publish artifact %q: %w", artifact.name, err), rollbackErr)
		}
	}

	for _, backup := range backups {
		if backup.backup == "" {
			continue
		}
		if err := root.Remove(backup.backup); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func restoreArtifactBackups(root *os.Root, backups []artifactBackup) error {
	var restoreErr error
	for i := len(backups) - 1; i >= 0; i-- {
		backup := backups[i]
		if !backup.hadOriginal {
			continue
		}
		if err := root.Rename(backup.backup, backup.name); err != nil {
			restoreErr = errors.Join(restoreErr, err)
		}
	}
	return restoreErr
}

func rollbackPublishedArtifacts(root *os.Root, artifacts []artifactSpec, backups []artifactBackup) error {
	var rollbackErr error
	for i := len(artifacts) - 1; i >= 0; i-- {
		if err := root.Remove(artifacts[i].name); err != nil && !os.IsNotExist(err) {
			rollbackErr = errors.Join(rollbackErr, err)
		}
		if backups[i].hadOriginal {
			if err := root.Rename(backups[i].backup, backups[i].name); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
	}
	return rollbackErr
}

func createTempRootFile(root *os.Root, prefix string) (string, *os.File, error) {
	var random [16]byte
	for attempt := 0; attempt < 10; attempt++ {
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, err
		}
		name := prefix + hex.EncodeToString(random[:])
		handle, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return name, handle, nil
		}
		if !os.IsExist(err) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("unable to create a unique temporary artifact")
}
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
