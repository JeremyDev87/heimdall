from __future__ import annotations

import argparse
import json
from pathlib import Path

from .contract import ContractError, load_spec
from .evaluator import evaluate
from .report import write_artifacts
from .snapshot import SnapshotError
from .status import EXIT_CODES, FinalStatus


def _print(payload: dict[str, object]) -> None:
    print(json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":")))


def _blocked(reason: str) -> int:
    _print({"reason": reason, "state": FinalStatus.BLOCKED.value})
    return EXIT_CODES[FinalStatus.BLOCKED]


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="heimdall", description="Evaluate a trusted local agent harness")
    subparsers = parser.add_subparsers(dest="command", required=True)
    validate = subparsers.add_parser("validate", help="validate an evaluation manifest")
    validate.add_argument("manifest")
    evaluate_parser = subparsers.add_parser("evaluate", help="run deterministic evaluation")
    evaluate_parser.add_argument("manifest")
    evaluate_parser.add_argument("--out", required=True)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = _parser().parse_args(argv)
    try:
        if args.command == "validate":
            load_spec(args.manifest)
            _print({"reason": "valid_manifest", "state": FinalStatus.PASS.value})
            return EXIT_CODES[FinalStatus.PASS]

        result = evaluate(args.manifest)
        out = Path(args.out).expanduser().resolve()
        if out == result.target_root or result.target_root in out.parents:
            return _blocked("invalid_output")
        write_artifacts(out, result.evidence, result.report, result.markdown)
        state = FinalStatus(result.report["state"])
        _print(
            {
                "evidence_digest": result.evidence["semantic_digest"],
                "report_digest": result.report["semantic_digest"],
                "state": state.value,
            }
        )
        return EXIT_CODES[state]
    except ContractError as error:
        return _blocked(error.code)
    except SnapshotError as error:
        return _blocked(error.code)
    except (OSError, ValueError):
        return _blocked("evaluation_unavailable")


def entrypoint() -> None:
    raise SystemExit(main())


if __name__ == "__main__":
    entrypoint()
