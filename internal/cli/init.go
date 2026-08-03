package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	heimdallassets "github.com/JeremyDev87/heimdall"
	"github.com/JeremyDev87/heimdall/internal/canonjson"
)

const commandArtifactPreset = "command-artifact"

type scaffoldFile struct {
	data []byte
	mode os.FileMode
}

func initHarness(args []string, stdout, stderr io.Writer) int {
	preset, target := commandArtifactPreset, "."
	separator := -1
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--":
			separator = index
			index = len(args)
		case "--preset", "--target":
			if index+1 >= len(args) {
				return usage(stderr)
			}
			value := args[index+1]
			if args[index] == "--preset" {
				preset = value
			} else {
				target = value
			}
			index++
		default:
			return usage(stderr)
		}
	}
	if separator < 0 || separator+1 >= len(args) {
		return usage(stderr)
	}
	command := args[separator+1:]
	if preset != commandArtifactPreset {
		return blocked(stdout, "unsupported_preset")
	}
	if len(command) > 32 {
		return blocked(stdout, "invalid_command")
	}
	for _, argument := range command {
		if argument == "" || len(argument) > 1024 || strings.ContainsRune(argument, '\x00') {
			return blocked(stdout, "invalid_command")
		}
	}
	files, err := renderCommandArtifact(target, command)
	if err != nil {
		return blocked(stdout, "scaffold_unavailable")
	}
	created, err := writeScaffold(target, files)
	if err != nil {
		return blocked(stdout, "scaffold_conflict")
	}
	reason := "scaffold_unchanged"
	if created {
		reason = "scaffold_created"
	}
	_ = canonjson.WriteLine(stdout, map[string]any{
		"manifest": ".heimdall-eval.yaml",
		"preset":   preset,
		"reason":   reason,
		"state":    "PASS",
	})
	return 0
}

func renderCommandArtifact(target string, command []string) (map[string]scaffoldFile, error) {
	absolute, err := filepath.Abs(expandHome(target))
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, os.ErrNotExist
	}
	data := struct {
		TargetID string
		Command  string
	}{
		TargetID: normalizedTargetID(filepath.Base(resolved)),
		Command:  shellCommand(command),
	}
	rendered := map[string]scaffoldFile{}
	for _, item := range []struct {
		source string
		target string
		mode   os.FileMode
	}{
		{"templates/command-artifact/eval.yaml.tmpl", ".heimdall-eval.yaml", 0o644},
		{"templates/command-artifact/verify-harness.sh.tmpl", ".heimdall/verify-harness.sh", 0o755},
	} {
		templateBytes, readErr := heimdallassets.FS.ReadFile(item.source)
		if readErr != nil {
			return nil, readErr
		}
		parsed, parseErr := template.New(filepath.Base(item.source)).Option("missingkey=error").Parse(string(templateBytes))
		if parseErr != nil {
			return nil, parseErr
		}
		var output bytes.Buffer
		if executeErr := parsed.Execute(&output, data); executeErr != nil {
			return nil, executeErr
		}
		rendered[item.target] = scaffoldFile{data: output.Bytes(), mode: item.mode}
	}
	policy, err := heimdallassets.FS.ReadFile("policies/harness-readiness-v1.yaml")
	if err != nil {
		return nil, err
	}
	rendered[".heimdall-policy.yaml"] = scaffoldFile{data: policy, mode: 0o644}
	return rendered, nil
}

func writeScaffold(target string, files map[string]scaffoldFile) (bool, error) {
	absolute, err := filepath.Abs(expandHome(target))
	if err != nil {
		return false, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return false, err
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return false, err
	}
	defer root.Close()
	scaffoldDir := ".heimdall"
	directoryExists := false
	if info, statErr := root.Lstat(scaffoldDir); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false, os.ErrExist
		}
		directoryExists = true
	} else if !os.IsNotExist(statErr) {
		return false, statErr
	}
	existing := 0
	for name, expected := range files {
		path := filepath.FromSlash(name)
		entryInfo, statErr := root.Lstat(path)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil || !entryInfo.Mode().IsRegular() || entryInfo.Mode()&os.ModeSymlink != 0 {
			return false, os.ErrExist
		}
		actual, readErr := root.ReadFile(path)
		if readErr != nil || !bytes.Equal(actual, expected.data) {
			return false, os.ErrExist
		}
		if expected.mode&0o111 != 0 && entryInfo.Mode().Perm()&0o111 == 0 {
			return false, os.ErrExist
		}
		existing++
	}
	if existing == len(files) {
		return false, nil
	}
	if existing != 0 {
		return false, os.ErrExist
	}
	if directoryExists {
		return false, os.ErrExist
	}
	if err := root.Mkdir(scaffoldDir, 0o755); err != nil {
		return false, err
	}
	scaffoldInfo, err := root.Lstat(scaffoldDir)
	if err != nil || !scaffoldInfo.IsDir() || scaffoldInfo.Mode()&os.ModeSymlink != 0 {
		return false, os.ErrExist
	}
	type publication struct {
		name string
		info os.FileInfo
	}
	published := []publication{}
	rollback := func() {
		for index := len(published) - 1; index >= 0; index-- {
			removeIfSame(root, published[index].name, published[index].info)
		}
		removeIfSame(root, scaffoldDir, scaffoldInfo)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		item := files[name]
		path := filepath.FromSlash(name)
		info, err := writeExclusive(root, path, item)
		if err != nil {
			rollback()
			return false, err
		}
		current, statErr := root.Lstat(path)
		if statErr != nil || !os.SameFile(info, current) {
			rollback()
			return false, os.ErrExist
		}
		published = append(published, publication{name: path, info: info})
	}
	return true, nil
}

func writeExclusive(root *os.Root, path string, file scaffoldFile) (os.FileInfo, error) {
	handle, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, file.mode)
	if err != nil {
		return nil, err
	}
	info, statErr := handle.Stat()
	if statErr != nil {
		_ = handle.Close()
		return nil, statErr
	}
	if _, err := handle.Write(file.data); err != nil {
		_ = handle.Close()
		removeIfSame(root, path, info)
		return nil, err
	}
	if err := handle.Close(); err != nil {
		removeIfSame(root, path, info)
		return nil, err
	}
	return info, nil
}

func removeIfSame(root *os.Root, path string, expected os.FileInfo) {
	current, err := root.Lstat(path)
	if err == nil && os.SameFile(expected, current) {
		_ = root.Remove(path)
	}
}

func normalizedTargetID(name string) string {
	var output strings.Builder
	previousDash := false
	for _, character := range strings.ToLower(name) {
		allowed := character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-'
		if !allowed {
			character = '-'
		}
		if character == '-' {
			if previousDash || output.Len() == 0 {
				continue
			}
			previousDash = true
		} else {
			previousDash = false
		}
		if output.Len()+1 > 64 {
			break
		}
		output.WriteRune(character)
	}
	id := strings.Trim(output.String(), "._-")
	if id == "" {
		return "local-harness"
	}
	first := id[0]
	if !((first >= 'a' && first <= 'z') || (first >= '0' && first <= '9')) {
		return "local-harness"
	}
	return id
}

func shellCommand(arguments []string) string {
	quoted := make([]string, len(arguments))
	for index, argument := range arguments {
		quoted[index] = "'" + strings.ReplaceAll(argument, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}
