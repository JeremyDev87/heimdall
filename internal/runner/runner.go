package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JeremyDev87/heimdall/internal/canonjson"
	"github.com/JeremyDev87/heimdall/internal/contract"
	"github.com/JeremyDev87/heimdall/internal/snapshot"
)

func Run(spec *contract.Spec, workspace string) (map[string]any, error) {
	before, err := snapshot.TreeDigest(spec.TargetRoot)
	if err != nil {
		return nil, err
	}
	targetCopy := filepath.Join(workspace, "target")
	if err := snapshot.CopyTarget(spec.TargetRoot, targetCopy); err != nil {
		return nil, err
	}
	home, temp := filepath.Join(targetCopy, ".heimdall-home"), filepath.Join(targetCopy, ".heimdall-tmp")
	if err := os.Mkdir(home, 0o700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(temp, 0o700); err != nil {
		return nil, err
	}
	cwd, err := containedPath(targetCopy, spec.Cwd)
	if err != nil {
		return nil, err
	}
	if info, statErr := os.Stat(cwd); statErr != nil || !info.IsDir() {
		return nil, os.ErrNotExist
	}

	environment := map[string]string{}
	for _, key := range []string{"PATH", "LANG", "LC_ALL"} {
		if value, ok := os.LookupEnv(key); ok {
			environment[key] = value
		}
	}
	environment["HOME"], environment["TMPDIR"], environment["PYTHONDONTWRITEBYTECODE"] = home, temp, "1"
	for key, value := range spec.Env {
		environment[key] = value
	}
	commandDigest, err := canonjson.Digest(map[string]any{"argv": spec.Argv, "cwd": spec.Cwd, "env": spec.Env})
	if err != nil {
		return nil, err
	}

	var stdout, stderr bytes.Buffer
	var exitCode any = nil
	timedOut, launchError := false, false
	command := exec.Command(spec.Argv[0], spec.Argv[1:]...)
	command.Dir = cwd
	command.Env = environmentList(environment)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := configureProcess(command); err != nil {
		launchError = true
	} else if err := command.Start(); err != nil {
		launchError = true
	} else {
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		timer := time.NewTimer(time.Duration(spec.TimeoutSeconds) * time.Second)
		select {
		case <-done:
			if !timer.Stop() {
				<-timer.C
			}
			exitCode = command.ProcessState.ExitCode()
		case <-timer.C:
			timedOut = true
			_ = terminateProcess(command)
			<-done
		}
	}

	outsideWrite := false
	workspaceEntries, err := os.ReadDir(workspace)
	if err != nil {
		return nil, err
	}
	for _, entry := range workspaceEntries {
		if entry.Name() != "target" {
			outsideWrite = true
			break
		}
	}
	checks, err := runChecks(targetCopy, spec.Checks)
	if err != nil {
		return nil, err
	}
	after, err := snapshot.TreeDigest(spec.TargetRoot)
	if err != nil {
		return nil, err
	}
	evidence := map[string]any{
		"schema_version": "1.0",
		"target":         map[string]any{"id": spec.TargetID, "digest_before": before, "digest_after": after, "no_write": before == after},
		"policy":         map[string]any{"id": spec.Policy["id"], "version": spec.Policy["version"], "digest": spec.PolicyDigest},
		"isolation":      map[string]any{"requested": spec.Isolation, "effective": "temp-copy-sanitized-env", "security_boundary": false},
		"execution":      map[string]any{"command_digest": commandDigest, "exit_code": exitCode, "timed_out": timedOut, "launch_error": launchError, "stdout": blob(stdout.Bytes()), "stderr": blob(stderr.Bytes())},
		"boundary":       map[string]any{"outside_workspace_write": outsideWrite}, "checks": checks,
	}
	digest, err := canonjson.Digest(evidence)
	if err != nil {
		return nil, err
	}
	evidence["semantic_digest"] = digest
	return evidence, nil
}

func runChecks(root string, checks []contract.Check) ([]any, error) {
	results := make([]map[string]any, 0, len(checks))
	for _, check := range checks {
		path, err := containedPath(root, check.Path)
		if err != nil {
			return nil, err
		}
		info, statErr := os.Stat(path)
		isFile := statErr == nil && info.Mode().IsRegular()
		exists := statErr == nil
		var artifact any = nil
		var data []byte
		if isFile {
			data, err = os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			sum := sha256.Sum256(data)
			artifact = hex.EncodeToString(sum[:])
		}
		status := "FAIL"
		switch check.Kind {
		case "file_exists":
			if isFile {
				status = "PASS"
			} else {
				status = "MISSING"
			}
		case "file_equals":
			if !isFile {
				status = "MISSING"
			} else if check.Expected != nil && string(data) == *check.Expected {
				status = "PASS"
			}
		case "path_absent":
			if !exists {
				status = "PASS"
			}
		}
		results = append(results, map[string]any{"id": check.ID, "kind": check.Kind, "status": status, "artifact_digest": artifact})
	}
	sort.Slice(results, func(i, j int) bool { return results[i]["id"].(string) < results[j]["id"].(string) })
	output := make([]any, len(results))
	for i := range results {
		output[i] = results[i]
	}
	return output, nil
}

func containedPath(root, relative string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Clean(filepath.Join(resolvedRoot, relative))
	rel, err := filepath.Rel(resolvedRoot, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", os.ErrPermission
	}
	if resolved, evalErr := filepath.EvalSymlinks(candidate); evalErr == nil {
		rel, err = filepath.Rel(resolvedRoot, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", os.ErrPermission
		}
		candidate = resolved
	}
	return candidate, nil
}
func environmentList(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}
func blob(data []byte) map[string]any {
	sum := sha256.Sum256(data)
	return map[string]any{"sha256": hex.EncodeToString(sum[:]), "size": len(data)}
}
