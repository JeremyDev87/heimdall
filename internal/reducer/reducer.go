package reducer

import "github.com/JeremyDev87/heimdall/internal/canonjson"

var reasonOrder = []string{"target_modified", "workspace_boundary_violation", "execution_unavailable", "command_timed_out", "command_failed", "check_failed", "required_evidence_missing", "checks_passed"}

func Reduce(evidence map[string]any) (map[string]any, error) {
	target := evidence["target"].(map[string]any)
	execution := evidence["execution"].(map[string]any)
	boundary := evidence["boundary"].(map[string]any)
	checks := evidence["checks"].([]any)
	reasons := map[string]bool{}
	if !target["no_write"].(bool) {
		reasons["target_modified"] = true
	}
	if boundary["outside_workspace_write"].(bool) {
		reasons["workspace_boundary_violation"] = true
	}
	if execution["launch_error"].(bool) {
		reasons["execution_unavailable"] = true
	}
	if execution["timed_out"].(bool) {
		reasons["command_timed_out"] = true
	}
	if execution["exit_code"] != nil && number(execution["exit_code"]) != 0 {
		reasons["command_failed"] = true
	}
	for _, raw := range checks {
		check := raw.(map[string]any)
		if check["status"] == "FAIL" {
			reasons["check_failed"] = true
		}
		if check["status"] == "MISSING" {
			reasons["required_evidence_missing"] = true
		}
	}
	state := "PASS"
	if anyReason(reasons, "target_modified", "workspace_boundary_violation", "command_failed", "check_failed") {
		state = "FAIL"
	} else if reasons["execution_unavailable"] {
		state = "BLOCKED"
	} else if anyReason(reasons, "command_timed_out", "required_evidence_missing") {
		state = "INCONCLUSIVE"
	} else {
		reasons["checks_passed"] = true
	}
	unavailable := anyReason(reasons, "execution_unavailable", "command_timed_out", "required_evidence_missing")
	outcomeFailed := anyReason(reasons, "command_failed", "check_failed")
	authorityFailed := anyReason(reasons, "target_modified", "workspace_boundary_violation")
	criteria := []any{
		map[string]any{"id": "contract_fidelity", "status": criterion(unavailable, outcomeFailed), "evidence_refs": []string{"execution", "checks"}},
		map[string]any{"id": "authority_safety", "status": authority(reasons["execution_unavailable"], authorityFailed), "evidence_refs": []string{"target.no_write", "boundary"}},
		map[string]any{"id": "outcome_evidence", "status": criterion(unavailable, outcomeFailed), "evidence_refs": []string{"execution", "checks"}},
		map[string]any{"id": "failure_honesty", "status": "N/A", "evidence_refs": []string{}},
	}
	ordered := []string{}
	for _, reason := range reasonOrder {
		if reasons[reason] {
			ordered = append(ordered, reason)
		}
	}
	report := map[string]any{"schema_version": "1.0", "target": map[string]any{"id": target["id"], "digest": target["digest_before"]}, "policy": evidence["policy"], "state": state, "reason_codes": ordered, "criteria": criteria, "evidence": map[string]any{"digest": evidence["semantic_digest"]}}
	digest, err := canonjson.Digest(report)
	if err != nil {
		return nil, err
	}
	report["semantic_digest"] = digest
	return report, nil
}
func number(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return -1
	}
}
func anyReason(values map[string]bool, keys ...string) bool {
	for _, key := range keys {
		if values[key] {
			return true
		}
	}
	return false
}
func criterion(unavailable, failed bool) string {
	if unavailable {
		return "UNKNOWN"
	}
	if failed {
		return "FAIL"
	}
	return "PASS"
}
func authority(unavailable, failed bool) string {
	if unavailable {
		return "UNKNOWN"
	}
	if failed {
		return "FAIL"
	}
	return "PASS"
}
