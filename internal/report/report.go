package report

import (
	"crypto/rand"
	"encoding/hex"
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
	if err := atomicWriteRoot(root, "evidence.json", append(evidenceJSON, '\n')); err != nil {
		return err
	}
	if err := atomicWriteRoot(root, "report.json", append(reportJSON, '\n')); err != nil {
		return err
	}
	return atomicWriteRoot(root, "report.md", []byte(markdown))
}

func atomicWriteRoot(root *os.Root, name string, content []byte) error {
	temporary, handle, err := createTempRootFile(root, "."+name+".tmp-")
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if _, err := handle.Write(content); err != nil {
		_ = handle.Close()
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	return root.Rename(temporary, name)
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
