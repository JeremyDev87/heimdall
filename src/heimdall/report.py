from __future__ import annotations

import json
import os
from pathlib import Path


def render_markdown(report: dict[str, object]) -> str:
    lines = [
        "# Heimdall Assessment",
        "",
        f"- State: `{report['state']}`",
        f"- Target: `{report['target']['id']}`",
        f"- Target digest: `{report['target']['digest']}`",
        f"- Policy: `{report['policy']['id']}@{report['policy']['version']}`",
        f"- Evidence digest: `{report['evidence']['digest']}`",
        f"- Reasons: `{', '.join(report['reason_codes'])}`",
        "",
        "| Criterion | Status |",
        "| --- | --- |",
    ]
    lines.extend(f"| {item['id']} | {item['status']} |" for item in report["criteria"] )
    lines.extend(
        [
            "",
            "> trusted-local execution uses a temporary copy and sanitized environment; it is not a security sandbox.",
            "",
        ]
    )
    return "\n".join(lines)


def _atomic_write(path: Path, content: str) -> None:
    temporary = path.with_name(f".{path.name}.tmp")
    temporary.write_text(content, encoding="utf-8")
    os.replace(temporary, path)


def write_artifacts(out: str | Path, evidence: dict[str, object], report: dict[str, object], markdown: str) -> None:
    directory = Path(out).expanduser().resolve()
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    _atomic_write(directory / "evidence.json", json.dumps(evidence, ensure_ascii=False, indent=2, sort_keys=True) + "\n")
    _atomic_write(directory / "report.json", json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n")
    _atomic_write(directory / "report.md", markdown)
