package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JeremyDev87/heimdall/internal/canonjson"
	"github.com/JeremyDev87/heimdall/internal/contract"
	"github.com/JeremyDev87/heimdall/internal/evaluator"
	"github.com/JeremyDev87/heimdall/internal/report"
	"github.com/JeremyDev87/heimdall/internal/snapshot"
)

const help = "usage: heimdall [-h] {validate,evaluate} ...\n\nEvaluate a trusted local agent harness\n\npositional arguments:\n  {validate,evaluate}\n    validate           validate an evaluation manifest\n    evaluate           run deterministic evaluation\n\noptions:\n  -h, --help            show this help message and exit\n"

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(stdout, help)
		return 0
	}
	if len(args) == 0 {
		fmt.Fprint(stderr, help)
		return 2
	}
	if len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
		switch args[0] {
		case "validate":
			fmt.Fprint(stdout, "usage: heimdall validate [-h] manifest\n")
			return 0
		case "evaluate":
			fmt.Fprint(stdout, "usage: heimdall evaluate [-h] --out OUT manifest\n")
			return 0
		}
	}
	switch args[0] {
	case "validate":
		if len(args) != 2 {
			return usage(stderr)
		}
		_, err := contract.LoadSpec(args[1])
		if err != nil {
			return blocked(stdout, errorCode(err))
		}
		_ = canonjson.WriteLine(stdout, map[string]any{"reason": "valid_manifest", "state": "PASS"})
		return 0
	case "evaluate":
		return evaluate(args[1:], stdout, stderr)
	default:
		return usage(stderr)
	}
}
func evaluate(args []string, stdout, stderr io.Writer) int {
	manifest, out := "", ""
	for index := 0; index < len(args); index++ {
		if args[index] == "--out" {
			if index+1 >= len(args) {
				return usage(stderr)
			}
			out = args[index+1]
			index++
			continue
		}
		if strings.HasPrefix(args[index], "-") || manifest != "" {
			return usage(stderr)
		}
		manifest = args[index]
	}
	if manifest == "" || out == "" {
		return usage(stderr)
	}
	artifacts, err := evaluator.Evaluate(manifest)
	if err != nil {
		return blocked(stdout, errorCode(err))
	}
	outPath, err := resolveForContainment(expandHome(out))
	if err != nil {
		return blocked(stdout, "invalid_output")
	}
	targetPath, err := resolveForContainment(artifacts.TargetRoot)
	if err != nil {
		return blocked(stdout, "evaluation_unavailable")
	}
	relative, relErr := filepath.Rel(targetPath, outPath)
	if relErr == nil && (relative == "." || (!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "..")) {
		return blocked(stdout, "invalid_output")
	}
	if err := report.WriteArtifacts(outPath, artifacts.Evidence, artifacts.Report, artifacts.Markdown); err != nil {
		return blocked(stdout, "evaluation_unavailable")
	}
	state := artifacts.Report["state"].(string)
	_ = canonjson.WriteLine(stdout, map[string]any{"evidence_digest": artifacts.Evidence["semantic_digest"], "report_digest": artifacts.Report["semantic_digest"], "state": state})
	return evaluator.ExitCode(state)
}
func blocked(stdout io.Writer, reason string) int {
	_ = canonjson.WriteLine(stdout, map[string]any{"reason": reason, "state": "BLOCKED"})
	return 2
}
func usage(stderr io.Writer) int { fmt.Fprint(stderr, help); return 2 }
func errorCode(err error) string {
	if code := contract.Code(err); code != "" {
		return code
	}
	if code := snapshot.Code(err); code != "" {
		return code
	}
	if errors.Is(err, os.ErrNotExist) {
		return "evaluation_unavailable"
	}
	return "evaluation_unavailable"
}
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func resolveForContainment(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(absolute)
	missing := []string{}
	for {
		resolved, evalErr := filepath.EvalSymlinks(current)
		if evalErr == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(evalErr) {
			return "", evalErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", evalErr
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
