import json
from pathlib import Path

from heimdall.cli import main

ROOT = Path(__file__).resolve().parents[1]


def test_cli_writes_content_light_artifacts(tmp_path, capsys):
    out = tmp_path / "out"
    code = main(["evaluate", str(ROOT / "fixtures/pass/eval.yaml"), "--out", str(out)])
    assert code == 0
    payload = json.loads(capsys.readouterr().out)
    assert payload["state"] == "PASS"
    assert sorted(path.name for path in out.iterdir()) == ["evidence.json", "report.json", "report.md"]


def test_cli_exit_codes_match_final_states(tmp_path, capsys):
    expected = {"pass": 0, "forbidden-write": 1, "false-pass": 3}
    for fixture, exit_code in expected.items():
        out = tmp_path / fixture
        assert main(["evaluate", str(ROOT / f"fixtures/{fixture}/eval.yaml"), "--out", str(out)]) == exit_code
        capsys.readouterr()


def test_invalid_manifest_is_safe_blocked_output(tmp_path, capsys):
    manifest = tmp_path / "broken.yaml"
    manifest.write_text("not: [valid", encoding="utf-8")
    code = main(["validate", str(manifest)])
    captured = capsys.readouterr()
    assert code == 2
    assert json.loads(captured.out) == {"reason": "invalid_manifest", "state": "BLOCKED"}
    assert str(tmp_path) not in captured.out


def test_output_inside_source_target_is_blocked_without_write(capsys):
    target = ROOT / "fixtures/pass/target"
    out = target / "reports"
    assert main(["evaluate", str(ROOT / "fixtures/pass/eval.yaml"), "--out", str(out)]) == 2
    assert json.loads(capsys.readouterr().out) == {"reason": "invalid_output", "state": "BLOCKED"}
    assert not out.exists()
