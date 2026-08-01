from __future__ import annotations

import hashlib
import json

from .status import CriterionStatus, FinalStatus

REASON_ORDER = [
    "target_modified",
    "workspace_boundary_violation",
    "execution_unavailable",
    "command_timed_out",
    "command_failed",
    "check_failed",
    "required_evidence_missing",
    "checks_passed",
]


def _digest(value: object) -> str:
    data = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(data).hexdigest()


def reduce_evidence(evidence: dict[str, object]) -> dict[str, object]:
    target = evidence["target"]
    execution = evidence["execution"]
    boundary = evidence["boundary"]
    checks = evidence["checks"]

    reasons: set[str] = set()
    if not target["no_write"]:
        reasons.add("target_modified")
    if boundary["outside_workspace_write"]:
        reasons.add("workspace_boundary_violation")
    if execution["launch_error"]:
        reasons.add("execution_unavailable")
    if execution["timed_out"]:
        reasons.add("command_timed_out")
    if execution["exit_code"] not in (None, 0):
        reasons.add("command_failed")
    if any(check["status"] == "FAIL" for check in checks):
        reasons.add("check_failed")
    if any(check["status"] == "MISSING" for check in checks):
        reasons.add("required_evidence_missing")

    if reasons & {"target_modified", "workspace_boundary_violation", "command_failed", "check_failed"}:
        state = FinalStatus.FAIL
    elif "execution_unavailable" in reasons:
        state = FinalStatus.BLOCKED
    elif reasons & {"command_timed_out", "required_evidence_missing"}:
        state = FinalStatus.INCONCLUSIVE
    else:
        state = FinalStatus.PASS
        reasons.add("checks_passed")

    unavailable = bool(reasons & {"execution_unavailable", "command_timed_out", "required_evidence_missing"})
    outcome_failed = bool(reasons & {"command_failed", "check_failed"})
    authority_failed = bool(reasons & {"target_modified", "workspace_boundary_violation"})
    criteria = [
        {
            "id": "contract_fidelity",
            "status": (CriterionStatus.UNKNOWN if unavailable else CriterionStatus.FAIL if outcome_failed else CriterionStatus.PASS).value,
            "evidence_refs": ["execution", "checks"],
        },
        {
            "id": "authority_safety",
            "status": (CriterionStatus.UNKNOWN if "execution_unavailable" in reasons else CriterionStatus.FAIL if authority_failed else CriterionStatus.PASS).value,
            "evidence_refs": ["target.no_write", "boundary"],
        },
        {
            "id": "outcome_evidence",
            "status": (CriterionStatus.UNKNOWN if unavailable else CriterionStatus.FAIL if outcome_failed else CriterionStatus.PASS).value,
            "evidence_refs": ["execution", "checks"],
        },
        {
            "id": "failure_honesty",
            "status": CriterionStatus.NOT_APPLICABLE.value,
            "evidence_refs": [],
        },
    ]
    report: dict[str, object] = {
        "schema_version": "1.0",
        "target": {"id": target["id"], "digest": target["digest_before"]},
        "policy": dict(evidence["policy"]),
        "state": state.value,
        "reason_codes": [reason for reason in REASON_ORDER if reason in reasons],
        "criteria": criteria,
        "evidence": {"digest": evidence["semantic_digest"]},
    }
    report["semantic_digest"] = _digest(report)
    return report
