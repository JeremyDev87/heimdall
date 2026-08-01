from __future__ import annotations

import hashlib
import json
import re
import sys
from dataclasses import dataclass
from importlib import resources
from pathlib import Path
from typing import Any

import yaml
from jsonschema import Draft202012Validator


class ContractError(ValueError):
    def __init__(self, code: str):
        self.code = code
        super().__init__(code)


class UniqueKeyLoader(yaml.SafeLoader):
    pass


def _construct_unique_mapping(loader: UniqueKeyLoader, node: yaml.MappingNode, deep: bool = False):
    mapping: dict[Any, Any] = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in mapping:
            raise ContractError("duplicate_key")
        mapping[key] = loader.construct_object(value_node, deep=deep)
    return mapping


UniqueKeyLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _construct_unique_mapping
)


@dataclass(frozen=True)
class EvalSpec:
    manifest_path: Path
    target_id: str
    target_root: Path
    policy: dict[str, Any]
    policy_path: Path
    policy_digest: str
    isolation: str
    argv: tuple[str, ...]
    cwd: str
    timeout_seconds: int
    env: dict[str, str]
    checks: tuple[dict[str, Any], ...]


def _load_yaml(path: Path) -> Any:
    try:
        return yaml.load(path.read_text(encoding="utf-8"), Loader=UniqueKeyLoader)
    except ContractError:
        raise
    except (OSError, UnicodeError, yaml.YAMLError) as error:
        raise ContractError("invalid_manifest") from error


def _schema_bytes(name: str) -> bytes:
    candidates = [
        Path(__file__).resolve().parents[2] / "schemas" / name,
        Path(sys.prefix) / "share" / "heimdall" / "schemas" / name,
    ]
    for candidate in candidates:
        if candidate.is_file():
            return candidate.read_bytes()
    try:
        return resources.files("heimdall").joinpath("schemas", name).read_bytes()
    except (FileNotFoundError, ModuleNotFoundError) as error:
        raise ContractError("schema_unavailable") from error


def _validate_schema(document: Any, schema_name: str) -> None:
    try:
        schema = json.loads(_schema_bytes(schema_name))
        Draft202012Validator(schema).validate(document)
    except ContractError:
        raise
    except Exception as error:
        raise ContractError("invalid_manifest") from error


def _safe_relative(value: str) -> str:
    path = Path(value)
    if path.is_absolute() or ".." in path.parts:
        raise ContractError("invalid_manifest")
    return value


def _load_policy(path: Path) -> dict[str, Any]:
    policy = _load_yaml(path)
    if not isinstance(policy, dict) or set(policy) != {
        "schema_version", "id", "version", "criteria"
    }:
        raise ContractError("invalid_policy")
    if policy["schema_version"] != "1.0" or not isinstance(policy["criteria"], list):
        raise ContractError("invalid_policy")
    expected_ids = {
        "contract_fidelity", "authority_safety", "outcome_evidence", "failure_honesty"
    }
    observed: set[str] = set()
    for criterion in policy["criteria"]:
        if (
            not isinstance(criterion, dict)
            or set(criterion) != {"id", "required"}
            or criterion.get("id") not in expected_ids
            or not isinstance(criterion.get("required"), bool)
            or criterion["id"] in observed
        ):
            raise ContractError("invalid_policy")
        observed.add(criterion["id"])
    if observed != expected_ids:
        raise ContractError("invalid_policy")
    return policy


def load_spec(path: str | Path) -> EvalSpec:
    manifest_path = Path(path).expanduser().resolve()
    document = _load_yaml(manifest_path)
    _validate_schema(document, "eval-spec.v1.json")

    target_root_value = _safe_relative(document["target"]["root"])
    cwd = _safe_relative(document["command"].get("cwd", "."))
    check_ids: set[str] = set()
    checks: list[dict[str, Any]] = []
    for raw in document["checks"]:
        check = dict(raw)
        _safe_relative(check["path"])
        if check["id"] in check_ids:
            raise ContractError("invalid_manifest")
        check_ids.add(check["id"])
        if check["kind"] == "file_equals" and "expected" not in check:
            raise ContractError("invalid_manifest")
        if check["kind"] != "file_equals" and "expected" in check:
            raise ContractError("invalid_manifest")
        checks.append(check)

    target_root = (manifest_path.parent / target_root_value).resolve()
    if not target_root.is_dir():
        raise ContractError("target_unavailable")

    policy_path_value = document["policy"]["path"]
    if Path(policy_path_value).is_absolute():
        raise ContractError("invalid_policy")
    policy_path = (manifest_path.parent / policy_path_value).resolve()
    if not policy_path.is_file():
        raise ContractError("invalid_policy")
    policy = _load_policy(policy_path)
    if (
        policy["id"] != document["policy"]["id"]
        or policy["version"] != document["policy"]["version"]
    ):
        raise ContractError("policy_mismatch")

    env = dict(document["command"].get("env", {}))
    for key in env:
        if re.fullmatch(r"[A-Z_][A-Z0-9_]{0,63}", key) is None:
            raise ContractError("invalid_manifest")

    return EvalSpec(
        manifest_path=manifest_path,
        target_id=document["target"]["id"],
        target_root=target_root,
        policy=policy,
        policy_path=policy_path,
        policy_digest=hashlib.sha256(policy_path.read_bytes()).hexdigest(),
        isolation=document["isolation"],
        argv=tuple(document["command"]["argv"]),
        cwd=cwd,
        timeout_seconds=document["command"]["timeout_seconds"],
        env=env,
        checks=tuple(checks),
    )
