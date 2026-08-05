package heimdallassets_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const (
	checkoutSHA   = "11d5960a326750d5838078e36cf38b85af677262"
	setupGoSHA    = "40f1582b2485089dde7abd97c1529aa768e1baff"
	setupNodeSHA  = "49933ea5288caeca8642d1e84afbd3f7d6820020"
	uploadSHA     = "ea165f8d65b6e75b540449e92b4886f43607fa02"
	goreleaserSHA = "e435ccd777264be153ace6237001ef4d979d3a7a"
)

type goreleaserConfig struct {
	Version     int    `yaml:"version"`
	ProjectName string `yaml:"project_name"`
	Metadata    struct {
		ModTimestamp string `yaml:"mod_timestamp"`
	} `yaml:"metadata"`
	Builds []struct {
		ID           string   `yaml:"id"`
		Main         string   `yaml:"main"`
		Binary       string   `yaml:"binary"`
		Goos         []string `yaml:"goos"`
		Goarch       []string `yaml:"goarch"`
		Flags        []string `yaml:"flags"`
		Ldflags      []string `yaml:"ldflags"`
		ModTimestamp string   `yaml:"mod_timestamp"`
		Ignore       []struct {
			Goos   string `yaml:"goos"`
			Goarch string `yaml:"goarch"`
		} `yaml:"ignore"`
	} `yaml:"builds"`
	Archives []struct {
		Formats      []string `yaml:"formats"`
		NameTemplate string   `yaml:"name_template"`
		BuildsInfo   struct {
			MTime string `yaml:"mtime"`
		} `yaml:"builds_info"`
		Files []string `yaml:"files"`
	} `yaml:"archives"`
	Checksum struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"checksum"`
	Release struct {
		Draft bool `yaml:"draft"`
	} `yaml:"release"`
}

type workflowConfig struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Uses  string         `yaml:"uses"`
	Steps []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Uses string `yaml:"uses"`
}

type workflowUse struct {
	Location string
	Ref      string
}

func TestReleasePackagingConfiguration(t *testing.T) {
	raw, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var config goreleaserConfig
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}
	if config.Version != 2 || config.ProjectName != "heimdall" {
		t.Fatalf("unexpected config identity: version=%d project=%q", config.Version, config.ProjectName)
	}
	if config.Metadata.ModTimestamp != "{{ .CommitTimestamp }}" {
		t.Fatalf("metadata mod timestamp is not commit-bound: %q", config.Metadata.ModTimestamp)
	}
	if len(config.Builds) != 1 {
		t.Fatalf("build count=%d, want 1", len(config.Builds))
	}
	build := config.Builds[0]
	if build.ID != "heimdall" || build.Main != "./cmd/heimdall" || build.Binary != "heimdall" {
		t.Fatalf("unexpected build identity: %#v", build)
	}
	if !slices.Equal(build.Goos, []string{"linux", "darwin"}) || !slices.Equal(build.Goarch, []string{"amd64", "arm64"}) {
		t.Fatalf("unexpected build axes: goos=%v goarch=%v", build.Goos, build.Goarch)
	}
	ignored := map[string]bool{}
	for _, pair := range build.Ignore {
		ignored[pair.Goos+"/"+pair.Goarch] = true
	}
	if len(ignored) != 2 || !ignored["linux/arm64"] || !ignored["darwin/amd64"] {
		t.Fatalf("release matrix must be exactly linux/amd64 and darwin/arm64; ignores=%v", ignored)
	}
	if !slices.Contains(build.Flags, "-trimpath") || build.ModTimestamp != "{{ .CommitTimestamp }}" {
		t.Fatalf("build reproducibility controls missing: flags=%v mod_timestamp=%q", build.Flags, build.ModTimestamp)
	}
	ldflags := strings.Join(build.Ldflags, " ")
	for _, marker := range []string{
		"internal/cli.Version={{ .Version }}",
		"internal/cli.Commit={{ .FullCommit }}",
		"internal/cli.BuildDate={{ .CommitDate }}",
	} {
		if !strings.Contains(ldflags, marker) {
			t.Errorf("ldflags missing %q", marker)
		}
	}
	if len(config.Archives) != 1 {
		t.Fatalf("archive count=%d, want 1", len(config.Archives))
	}
	archive := config.Archives[0]
	if !slices.Equal(archive.Formats, []string{"tar.gz"}) || archive.BuildsInfo.MTime != "{{ .CommitDate }}" {
		t.Fatalf("archive determinism controls missing: formats=%v mtime=%q", archive.Formats, archive.BuildsInfo.MTime)
	}
	for _, marker := range []string{"{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}", "LICENSE", "README.md"} {
		if marker == "LICENSE" || marker == "README.md" {
			if !slices.Contains(archive.Files, marker) {
				t.Errorf("archive files missing %q", marker)
			}
		} else if archive.NameTemplate != marker {
			t.Errorf("archive name template=%q, want %q", archive.NameTemplate, marker)
		}
	}
	if config.Checksum.NameTemplate != "checksums.txt" {
		t.Fatalf("checksum name=%q", config.Checksum.NameTemplate)
	}
	if !config.Release.Draft {
		t.Fatal("release must remain draft until uploaded assets pass verification")
	}
}

func TestReleaseWorkflowContract(t *testing.T) {
	assertFileMarkers(t, ".github/workflows/release.yml", []string{
		"tags:", `- "v*"`, "contents: write",
		"actions/checkout@" + checkoutSHA,
		"actions/setup-go@" + setupGoSHA,
		"goreleaser/goreleaser-action@" + goreleaserSHA,
		"version: v2.17.1", "args: release --clean",
		"secrets.RELEASE_ADMIN_TOKEN", "Administration:read", "immutable-releases", "default_branch", "gh release download",
		"--draft=false", "isImmutable",
	})
	raw, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	assertOrderedMarkers(t, text, []string{
		"name: Verify locally built assets",
		"name: Download and byte-compare draft assets",
		`gh release view "$GITHUB_REF_NAME" --json assets`,
		"gh release download",
		"cmp dist/checksums.txt",
		`cmp "$archive"`,
		"name: Verify byte-matched downloaded runtime",
		"name: Publish verified immutable release",
		"gh release edit",
	})
	runtimeStart := strings.Index(text, "name: Verify byte-matched downloaded runtime")
	publishStart := strings.Index(text, "name: Publish verified immutable release")
	if runtimeStart < 0 || publishStart <= runtimeStart {
		t.Fatal("downloaded runtime and publication steps are missing or misordered")
	}
	runtimeBlock := text[runtimeStart:publishStart]
	if strings.Contains(runtimeBlock, "GH_TOKEN") || !strings.Contains(runtimeBlock, "            full") {
		t.Fatal("byte-matched downloaded runtime must use full verification without GH_TOKEN")
	}
}

func TestSnapshotPackagingCIContract(t *testing.T) {
	assertFileMarkers(t, ".github/workflows/ci.yml", []string{
		"package-snapshot:",
		"actions/checkout@" + checkoutSHA,
		"actions/setup-go@" + setupGoSHA,
		"goreleaser/goreleaser-action@" + goreleaserSHA,
		"version: v2.17.1", "args: release --snapshot --clean",
		"bash scripts/verify-release.sh dist",
	})
}

func TestRealTargetWorkflowContract(t *testing.T) {
	uses := readWorkflowUses(t, ".github/workflows/real-target.yml")
	expected := map[string]string{
		"actions/checkout":        checkoutSHA,
		"actions/setup-go":        setupGoSHA,
		"actions/setup-node":      setupNodeSHA,
		"actions/upload-artifact": uploadSHA,
	}
	if err := validateExactGitHubUses(uses, expected); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowsPinEveryExternalUse(t *testing.T) {
	var paths []string
	for _, pattern := range []string{".github/workflows/*.yml", ".github/workflows/*.yaml"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, matches...)
	}
	if len(paths) == 0 {
		t.Fatal("no GitHub Actions workflows found")
	}
	for _, path := range paths {
		if err := validatePinnedUses(readWorkflowUses(t, path)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

func TestWorkflowUseValidationRejectsCommentDecoy(t *testing.T) {
	raw := []byte(fmt.Sprintf(`jobs:
  test:
    steps:
      # uses: actions/checkout@%s
      - uses: attacker/checkout@0000000000000000000000000000000000000000
`, checkoutSHA))
	uses, err := parseWorkflowUses(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateExactGitHubUses(uses, map[string]string{"actions/checkout": checkoutSHA}); err == nil {
		t.Fatal("comment decoy and wrong executable action unexpectedly passed exact-use validation")
	}
}

func TestWorkflowUseValidationRejectsFlowStyleMutableRefs(t *testing.T) {
	for name, raw := range map[string]string{
		"step": `jobs: {test: {steps: [{uses: actions/checkout@v4}]}}`,
		"job":  `jobs: {reusable: {uses: owner/repo/.github/workflows/ci.yml@v1}}`,
	} {
		t.Run(name, func(t *testing.T) {
			uses, err := parseWorkflowUses([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			if len(uses) != 1 {
				t.Fatalf("parsed uses=%d, want 1", len(uses))
			}
			if err := validatePinnedUses(uses); err == nil {
				t.Fatal("flow-style mutable external use unexpectedly passed validation")
			}
		})
	}
}

func TestReleaseVerifierContract(t *testing.T) {
	info, err := os.Stat("scripts/verify-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("scripts/verify-release.sh is not executable")
	}
	assertFileMarkers(t, "scripts/verify-release.sh", []string{
		"set -euo pipefail", "checksums.txt", "linux_amd64", "darwin_arm64",
		"heimdall version", "fixtures/pass/eval.yaml", "fixtures/false-pass/eval.yaml",
		"full|structural", "member.isfile()", `version_json=$("$binary" version)`,
	})
	raw, err := os.ReadFile("scripts/verify-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	validate := strings.Index(text, `python3 - "$archive_path"`)
	structural := strings.Index(text[validate:], `if [ "$verification_mode" = structural ]`)
	extract := strings.Index(text, `tar -xzf "$archive_path"`)
	if validate < 0 || structural < 0 || extract <= validate+structural {
		t.Fatal("archive names and types must be validated before structural return and extraction")
	}
}

func assertOrderedMarkers(t *testing.T, text string, markers []string) {
	t.Helper()
	position := -1
	for _, marker := range markers {
		relative := strings.Index(text[position+1:], marker)
		if relative < 0 {
			t.Fatalf("missing ordered marker %q", marker)
		}
		position += relative + 1
	}
}

func assertFileMarkers(t *testing.T, path string, markers []string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, marker := range markers {
		if !strings.Contains(text, marker) {
			t.Errorf("%s missing %q", path, marker)
		}
	}
}

func readWorkflowUses(t *testing.T, path string) []workflowUse {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	uses, err := parseWorkflowUses(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return uses
}

func parseWorkflowUses(raw []byte) ([]workflowUse, error) {
	var workflow workflowConfig
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		return nil, err
	}
	jobNames := make([]string, 0, len(workflow.Jobs))
	for name := range workflow.Jobs {
		jobNames = append(jobNames, name)
	}
	sort.Strings(jobNames)
	var uses []workflowUse
	for _, jobName := range jobNames {
		job := workflow.Jobs[jobName]
		if job.Uses != "" {
			uses = append(uses, workflowUse{Location: "jobs." + jobName + ".uses", Ref: job.Uses})
		}
		for index, step := range job.Steps {
			if step.Uses != "" {
				uses = append(uses, workflowUse{Location: fmt.Sprintf("jobs.%s.steps[%d].uses", jobName, index), Ref: step.Uses})
			}
		}
	}
	return uses, nil
}

func validatePinnedUses(uses []workflowUse) error {
	for _, use := range uses {
		switch {
		case strings.HasPrefix(use.Ref, "./"):
			continue
		case strings.HasPrefix(use.Ref, "docker://"):
			const marker = "@sha256:"
			index := strings.LastIndex(use.Ref, marker)
			if index <= len("docker://") || !isLowerHex(use.Ref[index+len(marker):], 64) {
				return fmt.Errorf("%s Docker action is not pinned to a sha256 digest: %s", use.Location, use.Ref)
			}
		default:
			if _, _, ok := splitPinnedGitHubUse(use.Ref); !ok {
				return fmt.Errorf("%s external GitHub action is not pinned to a full lowercase commit SHA: %s", use.Location, use.Ref)
			}
		}
	}
	return nil
}

func validateExactGitHubUses(uses []workflowUse, expected map[string]string) error {
	if err := validatePinnedUses(uses); err != nil {
		return err
	}
	actual := make(map[string]string)
	for _, use := range uses {
		if strings.HasPrefix(use.Ref, "./") || strings.HasPrefix(use.Ref, "docker://") {
			continue
		}
		action, revision, _ := splitPinnedGitHubUse(use.Ref)
		if _, duplicate := actual[action]; duplicate {
			return fmt.Errorf("duplicate external GitHub action: %s", action)
		}
		actual[action] = revision
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("external GitHub action count=%d, want %d", len(actual), len(expected))
	}
	actions := make([]string, 0, len(expected))
	for action := range expected {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	for _, action := range actions {
		if actual[action] != expected[action] {
			return fmt.Errorf("external GitHub action %s revision=%q, want %q", action, actual[action], expected[action])
		}
	}
	return nil
}

func splitPinnedGitHubUse(ref string) (string, string, bool) {
	index := strings.LastIndex(ref, "@")
	if index <= 0 || index == len(ref)-1 {
		return "", "", false
	}
	action, revision := ref[:index], ref[index+1:]
	parts := strings.Split(action, "/")
	if len(parts) < 2 || strings.Contains(action, "://") {
		return "", "", false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", "", false
		}
	}
	if !isLowerHex(revision, 40) {
		return "", "", false
	}
	return action, revision, true
}

func isLowerHex(value string, length int) bool {
	return len(value) == length && strings.Trim(value, "0123456789abcdef") == ""
}
