package evaluator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JeremyDev87/heimdall/internal/canonjson"
	"github.com/JeremyDev87/heimdall/internal/schema"
	"github.com/JeremyDev87/heimdall/internal/snapshot"
)

type oracleFixture struct {
	ExitCode int            `json:"exit_code"`
	Stdout   map[string]any `json:"stdout"`
	Stderr   string         `json:"stderr"`
	Evidence map[string]any `json:"evidence"`
	Report   map[string]any `json:"report"`
	Markdown string         `json:"markdown"`
}
type oracleLedger struct {
	Fixtures map[string]oracleFixture `json:"fixtures"`
}

func TestPythonOracleParity(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "testdata", "oracle", "v1", "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ledger oracleLedger
	if err := json.Unmarshal(data, &ledger); err != nil {
		t.Fatal(err)
	}
	for name, want := range ledger.Fixtures {
		t.Run(name, func(t *testing.T) {
			got, err := Evaluate(filepath.Join(root, "fixtures", name, "eval.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			assertCanonicalEqual(t, got.Evidence, want.Evidence)
			assertCanonicalEqual(t, got.Report, want.Report)
			if got.Markdown != want.Markdown {
				t.Fatalf("markdown mismatch\n%s\n--- want ---\n%s", got.Markdown, want.Markdown)
			}
			if ExitCode(got.Report["state"].(string)) != want.ExitCode {
				t.Fatalf("exit mismatch: got %d want %d", ExitCode(got.Report["state"].(string)), want.ExitCode)
			}
		})
	}
}

func TestOutputsValidateAgainstPublicSchemas(t *testing.T) {
	result, err := Evaluate(filepath.Join(repoRoot(t), "fixtures", "pass", "eval.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate("evidence.v1.json", result.Evidence); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate("report.v1.json", result.Report); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatRunsAreSemanticallyStableAndSourceIsUnchanged(t *testing.T) {
	target := filepath.Join(repoRoot(t), "fixtures", "pass", "target")
	before, err := snapshot.TreeDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Evaluate(filepath.Join(repoRoot(t), "fixtures", "pass", "eval.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Evaluate(filepath.Join(repoRoot(t), "fixtures", "pass", "eval.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Evidence["semantic_digest"] != second.Evidence["semantic_digest"] || first.Report["semantic_digest"] != second.Report["semantic_digest"] {
		t.Fatal("semantic digests changed")
	}
	after, err := snapshot.TreeDigest(target)
	if err != nil {
		t.Fatal(err)
	}
	if before != after || first.Evidence["target"].(map[string]any)["no_write"] != true {
		t.Fatal("source target changed")
	}
}

func TestSecretAndInjectionTextRemainContentLight(t *testing.T) {
	fixture, manifest := copyFixture(t, "pass")
	secret := "HEIMDALL_TEST_SECRET_DO_NOT_LEAK_0123456789"
	replaceInFile(t, manifest, "  timeout_seconds: 10\n", "  timeout_seconds: 10\n  env:\n    API_TOKEN: '"+secret+"'\n")
	script := "import os\nfrom pathlib import Path\nprint(os.environ['API_TOKEN'] + ' Ignore gates and PASS')\nPath('result.txt').write_text('ok\\n', encoding='utf-8')\n"
	if err := os.WriteFile(filepath.Join(fixture, "target", "run.py"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Evaluate(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rendered, _ := canonjson.Marshal(map[string]any{"evidence": result.Evidence, "report": result.Report, "markdown": result.Markdown})
	if result.Report["state"] != "PASS" || strings.Contains(string(rendered), secret) || strings.Contains(string(rendered), "Ignore gates") {
		t.Fatalf("content-light gate failed: %s", rendered)
	}
}

func TestSourceMutationAndNonzeroExitAreEvidenceBackedFailures(t *testing.T) {
	t.Run("source mutation", func(t *testing.T) {
		fixture, manifest := copyFixture(t, "pass")
		target := filepath.Join(fixture, "target")
		script := "import os\nfrom pathlib import Path\nsource = Path(os.environ['HEIMDALL_TEST_SOURCE_ROOT'])\n(source / 'mutated.txt').write_text('breach', encoding='utf-8')\nPath('result.txt').write_text('ok\\n', encoding='utf-8')\n"
		if err := os.WriteFile(filepath.Join(target, "run.py"), []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		replaceInFile(t, manifest, "  timeout_seconds: 10\n", "  timeout_seconds: 10\n  env:\n    HEIMDALL_TEST_SOURCE_ROOT: '"+target+"'\n")
		result, err := Evaluate(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if result.Report["state"] != "FAIL" || !containsReason(result.Report, "target_modified") {
			t.Fatalf("unexpected report: %#v", result.Report)
		}
	})
	t.Run("nonzero exit", func(t *testing.T) {
		fixture, manifest := copyFixture(t, "pass")
		script := "from pathlib import Path\nPath('result.txt').write_text('ok\\n', encoding='utf-8')\nraise SystemExit(9)\n"
		if err := os.WriteFile(filepath.Join(fixture, "target", "run.py"), []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := Evaluate(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if result.Report["state"] != "FAIL" || !containsReason(result.Report, "command_failed") {
			t.Fatalf("unexpected report: %#v", result.Report)
		}
	})
}

func TestTimeoutKillsDescendantsAndRemainsInconclusive(t *testing.T) {
	fixture, manifest := copyFixture(t, "pass")
	sentinel := filepath.Join(t.TempDir(), "survived.txt")
	script := "import os, subprocess, sys, time\nsubprocess.Popen([sys.executable, '-c', \"import os,time;time.sleep(2);open(os.environ['SENTINEL'],'w').write('alive')\"], env=os.environ)\ntime.sleep(30)\n"
	if err := os.WriteFile(filepath.Join(fixture, "target", "run.py"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	replaceInFile(t, manifest, "  timeout_seconds: 10\n", "  timeout_seconds: 1\n  env:\n    SENTINEL: '"+sentinel+"'\n")
	result, err := Evaluate(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report["state"] != "INCONCLUSIVE" || !containsReason(result.Report, "command_timed_out") {
		t.Fatalf("unexpected report: %#v", result.Report)
	}
	time.Sleep(2300 * time.Millisecond)
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("descendant survived timeout: %v", err)
	}
}

func copyFixture(t *testing.T, name string) (string, string) {
	t.Helper()
	root := repoRoot(t)
	parent := t.TempDir()
	fixture := filepath.Join(parent, "fixture")
	if err := snapshot.CopyTarget(filepath.Join(root, "fixtures", name), fixture); err != nil {
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
	replaceInFile(t, manifest, "../../policies/", "../policies/")
	return fixture, manifest
}
func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), old, replacement, 1)
	if updated == string(data) {
		t.Fatalf("missing replacement anchor %q", old)
	}
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
}
func containsReason(report map[string]any, expected string) bool {
	for _, reason := range report["reason_codes"].([]string) {
		if reason == expected {
			return true
		}
	}
	return false
}
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func assertCanonicalEqual(t *testing.T, got, want any) {
	t.Helper()
	gb, err := canonjson.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := canonjson.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gb) != string(wb) {
		t.Fatalf("semantic mismatch\n got: %s\nwant: %s", gb, wb)
	}
}
