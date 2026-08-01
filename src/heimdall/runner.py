from __future__ import annotations

import hashlib
import json
import os
import subprocess
from pathlib import Path

from .contract import EvalSpec
from .snapshot import copy_target, tree_digest


def _canonical_digest(value: object) -> str:
    data = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(data).hexdigest()


def _blob(value: bytes | str | None) -> dict[str, object]:
    if value is None:
        data = b""
    elif isinstance(value, bytes):
        data = value
    else:
        data = value.encode("utf-8", errors="replace")
    return {"sha256": hashlib.sha256(data).hexdigest(), "size": len(data)}


def _contained_path(root: Path, relative: str) -> Path:
    candidate = (root / relative).resolve()
    if not candidate.is_relative_to(root.resolve()):
        raise ValueError("path_outside_target")
    return candidate


def _run_checks(root: Path, checks: tuple[dict[str, object], ...]):
    results = []
    for check in checks:
        path = _contained_path(root, str(check["path"]))
        kind = str(check["kind"])
        artifact_digest = hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else None
        if kind == "file_exists":
            status = "PASS" if path.is_file() else "MISSING"
        elif kind == "file_equals":
            if not path.is_file():
                status = "MISSING"
            else:
                try:
                    actual = path.read_text(encoding="utf-8")
                except (OSError, UnicodeError):
                    status = "FAIL"
                else:
                    status = "PASS" if actual == check["expected"] else "FAIL"
        elif kind == "path_absent":
            status = "PASS" if not path.exists() else "FAIL"
        else:  # protected by the manifest schema
            status = "FAIL"
        results.append(
            {
                "id": str(check["id"]),
                "kind": kind,
                "status": status,
                "artifact_digest": artifact_digest,
            }
        )
    return sorted(results, key=lambda item: item["id"].encode("utf-8"))


def run(spec: EvalSpec, workspace: Path) -> dict[str, object]:
    before = tree_digest(spec.target_root)
    target_copy = workspace / "target"
    copy_target(spec.target_root, target_copy)
    home = target_copy / ".heimdall-home"
    temp = target_copy / ".heimdall-tmp"
    home.mkdir()
    temp.mkdir()

    cwd = _contained_path(target_copy, spec.cwd)
    if not cwd.is_dir():
        raise ValueError("cwd_unavailable")

    inherited = {key: os.environ[key] for key in ("PATH", "LANG", "LC_ALL") if key in os.environ}
    environment = {
        **inherited,
        "HOME": str(home),
        "TMPDIR": str(temp),
        "PYTHONDONTWRITEBYTECODE": "1",
        **spec.env,
    }
    command_digest = _canonical_digest(
        {"argv": list(spec.argv), "cwd": spec.cwd, "env": dict(sorted(spec.env.items()))}
    )
    exit_code = None
    timed_out = False
    launch_error = False
    stdout: bytes | str | None = b""
    stderr: bytes | str | None = b""
    try:
        completed = subprocess.run(
            list(spec.argv),
            cwd=cwd,
            env=environment,
            capture_output=True,
            check=False,
            timeout=spec.timeout_seconds,
        )
        exit_code = completed.returncode
        stdout = completed.stdout
        stderr = completed.stderr
    except subprocess.TimeoutExpired as error:
        timed_out = True
        stdout = error.stdout
        stderr = error.stderr
    except OSError:
        launch_error = True

    outside_workspace_write = any(path.name != "target" for path in workspace.iterdir())
    checks = _run_checks(target_copy, spec.checks)
    after = tree_digest(spec.target_root)
    evidence: dict[str, object] = {
        "schema_version": "1.0",
        "target": {
            "id": spec.target_id,
            "digest_before": before,
            "digest_after": after,
            "no_write": before == after,
        },
        "policy": {
            "id": spec.policy["id"],
            "version": spec.policy["version"],
            "digest": spec.policy_digest,
        },
        "isolation": {
            "requested": spec.isolation,
            "effective": "temp-copy-sanitized-env",
            "security_boundary": False,
        },
        "execution": {
            "command_digest": command_digest,
            "exit_code": exit_code,
            "timed_out": timed_out,
            "launch_error": launch_error,
            "stdout": _blob(stdout),
            "stderr": _blob(stderr),
        },
        "boundary": {"outside_workspace_write": outside_workspace_write},
        "checks": checks,
    }
    evidence["semantic_digest"] = _canonical_digest(evidence)
    return evidence
