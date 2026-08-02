package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/JeremyDev87/heimdall/internal/schema"
	"go.yaml.in/yaml/v3"
)

type Error struct{ code string }

func (err *Error) Error() string { return err.code }
func fail(code string) error     { return &Error{code: code} }
func Code(err error) string {
	var contractError *Error
	if errors.As(err, &contractError) {
		return contractError.code
	}
	return ""
}

type Check struct {
	ID       string
	Kind     string
	Path     string
	Expected *string
}

type Spec struct {
	ManifestPath   string
	TargetID       string
	TargetRoot     string
	Policy         map[string]any
	PolicyPath     string
	PolicyDigest   string
	Isolation      string
	Argv           []string
	Cwd            string
	TimeoutSeconds int
	Env            map[string]string
	Checks         []Check
}

type wireSpec struct {
	SchemaVersion string `json:"schema_version"`
	Target        struct {
		ID   string `json:"id"`
		Root string `json:"root"`
	} `json:"target"`
	Policy struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Path    string `json:"path"`
	} `json:"policy"`
	Isolation string `json:"isolation"`
	Command   struct {
		Argv           []string          `json:"argv"`
		Cwd            string            `json:"cwd"`
		TimeoutSeconds int               `json:"timeout_seconds"`
		Env            map[string]string `json:"env"`
	} `json:"command"`
	Checks []struct {
		ID       string  `json:"id"`
		Kind     string  `json:"kind"`
		Path     string  `json:"path"`
		Expected *string `json:"expected"`
	} `json:"checks"`
}

var envName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)

func LoadSpec(path string) (*Spec, error) {
	manifestPath, err := filepath.Abs(expandHome(path))
	if err != nil {
		return nil, fail("invalid_manifest")
	}
	manifestPath, err = filepath.EvalSymlinks(manifestPath)
	if err != nil {
		return nil, fail("invalid_manifest")
	}
	document, _, err := loadYAML(manifestPath)
	if err != nil {
		return nil, err
	}
	if err := schema.Validate("eval-spec.v1.json", document); err != nil {
		return nil, fail("invalid_manifest")
	}
	data, err := json.Marshal(document)
	if err != nil {
		return nil, fail("invalid_manifest")
	}
	var wire wireSpec
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fail("invalid_manifest")
	}

	if err := safeRelative(wire.Target.Root); err != nil {
		return nil, err
	}
	cwd := wire.Command.Cwd
	if cwd == "" {
		cwd = "."
	}
	if err := safeRelative(cwd); err != nil {
		return nil, err
	}
	checkIDs := map[string]bool{}
	checks := make([]Check, 0, len(wire.Checks))
	for _, raw := range wire.Checks {
		if err := safeRelative(raw.Path); err != nil {
			return nil, err
		}
		if checkIDs[raw.ID] {
			return nil, fail("invalid_manifest")
		}
		checkIDs[raw.ID] = true
		if raw.Kind == "file_equals" && raw.Expected == nil {
			return nil, fail("invalid_manifest")
		}
		if raw.Kind != "file_equals" && raw.Expected != nil {
			return nil, fail("invalid_manifest")
		}
		checks = append(checks, Check{ID: raw.ID, Kind: raw.Kind, Path: raw.Path, Expected: raw.Expected})
	}

	targetRoot := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), wire.Target.Root))
	targetRoot, err = filepath.EvalSymlinks(targetRoot)
	if err != nil {
		return nil, fail("target_unavailable")
	}
	info, err := os.Stat(targetRoot)
	if err != nil || !info.IsDir() {
		return nil, fail("target_unavailable")
	}
	if filepath.IsAbs(wire.Policy.Path) {
		return nil, fail("invalid_policy")
	}
	policyPath := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), wire.Policy.Path))
	policyPath, err = filepath.EvalSymlinks(policyPath)
	if err != nil {
		return nil, fail("invalid_policy")
	}
	info, err = os.Stat(policyPath)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fail("invalid_policy")
	}
	policy, policyBytes, err := loadPolicy(policyPath)
	if err != nil {
		return nil, err
	}
	if policy["id"] != wire.Policy.ID || policy["version"] != wire.Policy.Version {
		return nil, fail("policy_mismatch")
	}
	for key := range wire.Command.Env {
		if !envName.MatchString(key) {
			return nil, fail("invalid_manifest")
		}
	}
	if wire.Command.Env == nil {
		wire.Command.Env = map[string]string{}
	}
	sum := sha256.Sum256(policyBytes)
	return &Spec{
		ManifestPath: manifestPath, TargetID: wire.Target.ID, TargetRoot: targetRoot,
		Policy: policy, PolicyPath: policyPath, PolicyDigest: hex.EncodeToString(sum[:]),
		Isolation: wire.Isolation, Argv: wire.Command.Argv, Cwd: cwd,
		TimeoutSeconds: wire.Command.TimeoutSeconds, Env: wire.Command.Env, Checks: checks,
	}, nil
}

func loadPolicy(path string) (map[string]any, []byte, error) {
	document, data, err := loadYAML(path)
	if err != nil {
		return nil, nil, err
	}
	policy, ok := document.(map[string]any)
	if !ok || !exactKeys(policy, "schema_version", "id", "version", "criteria") {
		return nil, nil, fail("invalid_policy")
	}
	criteria, ok := policy["criteria"].([]any)
	if policy["schema_version"] != "1.0" || !ok {
		return nil, nil, fail("invalid_policy")
	}
	expected := map[string]bool{"contract_fidelity": true, "authority_safety": true, "outcome_evidence": true, "failure_honesty": true}
	observed := map[string]bool{}
	for _, raw := range criteria {
		criterion, ok := raw.(map[string]any)
		if !ok || !exactKeys(criterion, "id", "required") {
			return nil, nil, fail("invalid_policy")
		}
		id, idOK := criterion["id"].(string)
		_, requiredOK := criterion["required"].(bool)
		if !idOK || !requiredOK || !expected[id] || observed[id] {
			return nil, nil, fail("invalid_policy")
		}
		observed[id] = true
	}
	if len(observed) != len(expected) {
		return nil, nil, fail("invalid_policy")
	}
	return policy, data, nil
}

func loadYAML(path string) (any, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fail("invalid_manifest")
	}
	var node yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&node); err != nil {
		return nil, nil, fail("invalid_manifest")
	}
	if len(node.Content) == 0 {
		return nil, nil, fail("invalid_manifest")
	}
	if duplicateKeys(node.Content[0]) {
		return nil, nil, fail("duplicate_key")
	}
	var raw any
	if err := node.Content[0].Decode(&raw); err != nil {
		return nil, nil, fail("invalid_manifest")
	}
	normalized, err := normalizeYAML(raw)
	if err != nil {
		return nil, nil, fail("invalid_manifest")
	}
	jsonBytes, err := json.Marshal(normalized)
	if err != nil {
		return nil, nil, fail("invalid_manifest")
	}
	var document any
	jsonDecoder := json.NewDecoder(bytes.NewReader(jsonBytes))
	jsonDecoder.UseNumber()
	if err := jsonDecoder.Decode(&document); err != nil {
		return nil, nil, fail("invalid_manifest")
	}
	return document, data, nil
}

func duplicateKeys(node *yaml.Node) bool {
	if node.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || seen[key.Value] {
				return true
			}
			seen[key.Value] = true
			if duplicateKeys(node.Content[index+1]) {
				return true
			}
		}
		return false
	}
	for _, child := range node.Content {
		if duplicateKeys(child) {
			return true
		}
	}
	return false
}

func normalizeYAML(value any) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized, err := normalizeYAML(item)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case map[any]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			text, ok := key.(string)
			if !ok {
				return nil, fail("invalid_manifest")
			}
			normalized, err := normalizeYAML(item)
			if err != nil {
				return nil, err
			}
			result[text] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			normalized, err := normalizeYAML(item)
			if err != nil {
				return nil, err
			}
			result[index] = normalized
		}
		return result, nil
	default:
		return value, nil
	}
}

func safeRelative(value string) error {
	if filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return fail("invalid_manifest")
	}
	for _, part := range strings.FieldsFunc(filepath.ToSlash(value), func(r rune) bool { return r == '/' }) {
		if part == ".." {
			return fail("invalid_manifest")
		}
	}
	return nil
}

func exactKeys(value map[string]any, expected ...string) bool {
	if len(value) != len(expected) {
		return false
	}
	sort.Strings(expected)
	actual := make([]string, 0, len(value))
	for key := range value {
		actual = append(actual, key)
	}
	sort.Strings(actual)
	for index := range actual {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
