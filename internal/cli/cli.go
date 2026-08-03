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

var Version = "dev"
var Commit = "unknown"
var BuildDate = "unknown"

const help = "usage: heimdall [-h] {init,check,version,validate,evaluate} ...\n\nEvaluate a trusted local agent harness\n\npositional arguments:\n  {init,check,version,validate,evaluate}\n    init               create a reviewable command-artifact scaffold\n    check              validate and evaluate a harness\n    version            print build provenance\n    validate           validate an evaluation manifest\n    evaluate           run deterministic evaluation\n\noptions:\n  -h, --help            show this help message and exit\n"

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
		case "init":
			fmt.Fprint(stdout, "usage: heimdall init [-h] [--preset command-artifact] [--target TARGET] -- COMMAND [ARG ...]\n")
			return 0
		case "check":
			fmt.Fprint(stdout, "usage: heimdall check [-h] [--out OUT] [manifest]\n")
			return 0
		case "version":
			fmt.Fprint(stdout, "usage: heimdall version [-h]\n")
			return 0
		case "validate":
			fmt.Fprint(stdout, "usage: heimdall validate [-h] manifest\n")
			return 0
		case "evaluate":
			fmt.Fprint(stdout, "usage: heimdall evaluate [-h] --out OUT manifest\n")
			return 0
		}
	}
	switch args[0] {
	case "init":
		return initHarness(args[1:], stdout, stderr)
	case "check":
		return check(args[1:], stdout, stderr)
	case "version":
		if len(args) != 1 {
			return usage(stderr)
		}
		_ = canonjson.WriteLine(stdout, map[string]any{"build_date": BuildDate, "commit": Commit, "version": Version})
		return 0
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
			if index+1 >= len(args) || out != "" {
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
	code, _ := runEvaluation(manifest, out, false, stdout)
	return code
}

func check(args []string, stdout, stderr io.Writer) int {
	manifest, out := ".heimdall-eval.yaml", ""
	manifestSet := false
	for index := 0; index < len(args); index++ {
		if args[index] == "--out" {
			if index+1 >= len(args) || out != "" {
				return usage(stderr)
			}
			out = args[index+1]
			index++
			continue
		}
		if strings.HasPrefix(args[index], "-") || manifestSet {
			return usage(stderr)
		}
		manifest, manifestSet = args[index], true
	}
	temporary := false
	if out == "" {
		var err error
		out, err = os.MkdirTemp("", "heimdall-artifacts-")
		if err != nil {
			return blocked(stdout, "invalid_output")
		}
		temporary = true
	}
	code, wrote := runEvaluation(manifest, out, true, stdout)
	if temporary && !wrote {
		_ = os.RemoveAll(out)
	}
	return code
}

func runEvaluation(manifest, out string, includeOutput bool, stdout io.Writer) (int, bool) {
	spec, err := contract.LoadSpec(manifest)
	if err != nil {
		return blocked(stdout, errorCode(err)), false
	}
	outPath, err := resolveForContainment(expandHome(out))
	if err != nil {
		return blocked(stdout, "invalid_output"), false
	}
	targetPath, err := resolveForContainment(spec.TargetRoot)
	if err != nil {
		return blocked(stdout, "evaluation_unavailable"), false
	}
	relative, relErr := filepath.Rel(targetPath, outPath)
	if relErr == nil && (relative == "." || (!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "..")) {
		return blocked(stdout, "invalid_output"), false
	}
	if err := os.MkdirAll(outPath, 0o700); err != nil {
		return blocked(stdout, "invalid_output"), false
	}
	outputIdentity, err := os.Lstat(outPath)
	if err != nil || !outputIdentity.IsDir() || outputIdentity.Mode()&os.ModeSymlink != 0 {
		return blocked(stdout, "invalid_output"), false
	}
	outputRoot, err := os.OpenRoot(outPath)
	if err != nil {
		return blocked(stdout, "invalid_output"), false
	}
	defer outputRoot.Close()
	rootIdentity, err := outputRoot.Stat(".")
	if err != nil || !os.SameFile(outputIdentity, rootIdentity) {
		return blocked(stdout, "invalid_output"), false
	}
	artifacts, err := evaluator.EvaluateSpec(spec)
	if err != nil {
		return blocked(stdout, errorCode(err)), false
	}
	currentOutput, err := os.Lstat(outPath)
	currentRoot, rootErr := outputRoot.Stat(".")
	if err != nil || rootErr != nil || !currentOutput.IsDir() || currentOutput.Mode()&os.ModeSymlink != 0 || !os.SameFile(outputIdentity, currentOutput) || !os.SameFile(outputIdentity, currentRoot) {
		return blocked(stdout, "invalid_output"), false
	}
	if err := report.WriteArtifactsRoot(outputRoot, artifacts.Evidence, artifacts.Report, artifacts.Markdown); err != nil {
		return blocked(stdout, "evaluation_unavailable"), false
	}
	state := artifacts.Report["state"].(string)
	payload := map[string]any{
		"evidence_digest": artifacts.Evidence["semantic_digest"],
		"report_digest":   artifacts.Report["semantic_digest"],
		"state":           state,
	}
	if includeOutput {
		payload["artifacts_dir"] = outPath
	}
	_ = canonjson.WriteLine(stdout, payload)
	return evaluator.ExitCode(state), true
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
