package report

import (
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
	evidenceJSON, err := canonjson.MarshalIndent(evidence)
	if err != nil {
		return err
	}
	reportJSON, err := canonjson.MarshalIndent(assessment)
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(directory, "evidence.json"), append(evidenceJSON, '\n')); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(directory, "report.json"), append(reportJSON, '\n')); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(directory, "report.md"), []byte(markdown))
}
func atomicWrite(path string, content []byte) error {
	handle, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporary := handle.Name()
	defer os.Remove(temporary)
	if _, err := handle.Write(content); err != nil {
		_ = handle.Close()
		return err
	}
	if err := handle.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
