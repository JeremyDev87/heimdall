package heimdallassets_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type actionInput struct {
	Required bool   `yaml:"required"`
	Default  string `yaml:"default"`
}

type actionOutput struct {
	Description string `yaml:"description"`
	Value       string `yaml:"value"`
}

type actionStep struct {
	ID    string            `yaml:"id"`
	Uses  string            `yaml:"uses"`
	Shell string            `yaml:"shell"`
	Run   string            `yaml:"run"`
	Env   map[string]string `yaml:"env"`
}

type actionMetadata struct {
	Inputs  map[string]actionInput  `yaml:"inputs"`
	Outputs map[string]actionOutput `yaml:"outputs"`
	Runs    struct {
		Using string       `yaml:"using"`
		Steps []actionStep `yaml:"steps"`
	} `yaml:"runs"`
}

func TestCompositeActionContract(t *testing.T) {
	raw, err := os.ReadFile("action.yml")
	if err != nil {
		t.Fatal(err)
	}
	var metadata actionMetadata
	if err := yaml.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("parse action metadata: %v", err)
	}
	if metadata.Runs.Using != "composite" || len(metadata.Runs.Steps) != 1 {
		t.Fatalf("unexpected composite action runtime: %#v", metadata.Runs)
	}
	if !slices.Equal(sortedKeys(metadata.Inputs), []string{"manifest"}) {
		t.Fatalf("inputs=%v, want only manifest", sortedKeys(metadata.Inputs))
	}
	if metadata.Inputs["manifest"].Required || metadata.Inputs["manifest"].Default != ".heimdall-eval.yaml" {
		t.Fatalf("manifest input is not optional with the fixed default: %#v", metadata.Inputs["manifest"])
	}
	expectedOutputs := []string{"artifacts-dir", "binary-commit", "binary-path", "binary-version", "evidence-digest", "exit-code", "report-digest", "state"}
	if !slices.Equal(sortedKeys(metadata.Outputs), expectedOutputs) {
		t.Fatalf("outputs=%v, want %v", sortedKeys(metadata.Outputs), expectedOutputs)
	}
	step := metadata.Runs.Steps[0]
	if step.ID != "evaluate" || step.Shell != "bash" || !strings.Contains(step.Run, "run-action.sh") {
		t.Fatalf("unexpected action step: %#v", step)
	}
	for _, name := range []string{"INPUT_MANIFEST", "GITHUB_ACTION_PATH", "GITHUB_WORKSPACE", "RUNNER_TEMP", "RUNNER_OS", "RUNNER_ARCH"} {
		if step.Env[name] == "" {
			t.Fatalf("action step does not pass %s through env", name)
		}
	}
}

func TestCompositeActionPinsReleaseAndExposesOnlyContractInputs(t *testing.T) {
	raw, err := os.ReadFile("scripts/run-action.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, marker := range []string{
		"RELEASE_VERSION='0.1.0'",
		"RELEASE_COMMIT='1cc04368aebe25d459cc65796855a9f3e9ce3338'",
		"55700942cfec80c2c00f3a21c0c0ad3bb4fe5b8d09ef5f362b7b9f8a3acc2957",
		"883a564c95b58117d6803ea058ea54a11cfac19b2af17044c26306b40fc218db",
		"https://github.com/JeremyDev87/heimdall/releases/download/v0.1.0/",
		"ACTION_ERROR",
		"exit \"$verdict\"",
	} {
		if !strings.Contains(text, marker) {
			t.Errorf("run-action.sh missing contract marker %q", marker)
		}
	}
	for _, forbidden := range []string{"INPUT_URL", "INPUT_VERSION", "INPUT_REPOSITORY", "INPUT_COMMAND", "INPUT_OUTPUT"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("run-action.sh exposes forbidden input %q", forbidden)
		}
	}
}

func TestCompositeConsumerLaneUsesLocalActionWithoutGoSetup(t *testing.T) {
	raw, err := os.ReadFile(".github/workflows/action.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "uses: ./") {
		t.Fatal("consumer lane does not exercise uses: ./")
	}
	if strings.Contains(text, "setup-go") || strings.Contains(text, "go test") || strings.Contains(text, "go build") {
		t.Fatal("consumer lane installs or invokes the Go toolchain")
	}
	for _, marker := range []string{"fixtures/pass/eval.yaml", "fixtures/forbidden-write/eval.yaml", "fixtures/false-pass/eval.yaml", "fixtures/blocked/eval.yaml", "continue-on-error: true"} {
		if !strings.Contains(text, marker) {
			t.Errorf("consumer lane missing %q", marker)
		}
	}
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
