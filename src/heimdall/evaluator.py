from __future__ import annotations

import tempfile
from dataclasses import dataclass
from pathlib import Path

from .contract import EvalSpec, load_spec
from .reducer import reduce_evidence
from .report import render_markdown
from .runner import run


@dataclass(frozen=True)
class EvaluationArtifacts:
    evidence: dict[str, object]
    report: dict[str, object]
    markdown: str
    target_root: Path


def evaluate(manifest: str | Path) -> EvaluationArtifacts:
    spec: EvalSpec = load_spec(manifest)
    with tempfile.TemporaryDirectory(prefix="heimdall-") as temporary:
        evidence = run(spec, Path(temporary))
    report = reduce_evidence(evidence)
    return EvaluationArtifacts(
        evidence=evidence,
        report=report,
        markdown=render_markdown(report),
        target_root=spec.target_root,
    )
