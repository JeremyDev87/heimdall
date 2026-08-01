from pathlib import Path

import pytest

from heimdall.contract import ContractError, load_spec

ROOT = Path(__file__).resolve().parents[1]


def test_valid_manifest_loads_strict_contract():
    spec = load_spec(ROOT / "fixtures/pass/eval.yaml")
    assert spec.target_id == "pass"
    assert spec.isolation == "trusted-local"
    assert spec.policy["id"] == "harness-readiness"


def test_unknown_manifest_key_fails_closed(tmp_path):
    manifest = tmp_path / "eval.yaml"
    manifest.write_text("schema_version: '1.0'\nunexpected: true\n", encoding="utf-8")
    with pytest.raises(ContractError) as error:
        load_spec(manifest)
    assert error.value.code == "invalid_manifest"
    assert str(tmp_path) not in str(error.value)


def test_duplicate_yaml_key_fails_closed(tmp_path):
    manifest = tmp_path / "eval.yaml"
    manifest.write_text("schema_version: '1.0'\nschema_version: '1.0'\n", encoding="utf-8")
    with pytest.raises(ContractError) as error:
        load_spec(manifest)
    assert error.value.code == "duplicate_key"
