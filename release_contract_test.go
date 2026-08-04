package heimdallassets_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const (
	checkoutSHA   = "11d5960a326750d5838078e36cf38b85af677262"
	setupGoSHA    = "40f1582b2485089dde7abd97c1529aa768e1baff"
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

func TestReleaseWorkflowsPinEveryExternalAction(t *testing.T) {
	for _, path := range []string{".github/workflows/ci.yml", ".github/workflows/release.yml"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for lineNumber, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "uses:") && !strings.HasPrefix(line, "- uses:") {
				continue
			}
			uses := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			uses = strings.TrimSpace(strings.TrimPrefix(uses, "uses:"))
			parts := strings.SplitN(uses, "@", 2)
			if len(parts) != 2 || len(parts[1]) != 40 || strings.Trim(parts[1], "0123456789abcdef") != "" {
				t.Errorf("%s:%d external action is not pinned to a full lowercase commit SHA: %s", path, lineNumber+1, line)
			}
		}
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
