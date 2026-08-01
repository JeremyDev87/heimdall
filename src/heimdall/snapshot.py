from __future__ import annotations

import hashlib
import shutil
from pathlib import Path

MAX_FILES = 2_000
MAX_FILE_BYTES = 8 * 1024 * 1024
MAX_TOTAL_BYTES = 32 * 1024 * 1024
IGNORED_PARTS = {".git", ".venv", ".pytest_cache", ".ruff_cache", "__pycache__"}


class SnapshotError(RuntimeError):
    def __init__(self, code: str):
        self.code = code
        super().__init__(code)


def _entries(root: Path):
    entries = []
    for path in root.rglob("*"):
        relative = path.relative_to(root)
        if any(part in IGNORED_PARTS for part in relative.parts):
            continue
        entries.append((relative, path))
    return sorted(entries, key=lambda item: item[0].as_posix().encode("utf-8"))


def tree_digest(root: str | Path) -> str:
    root_path = Path(root).resolve()
    if not root_path.is_dir():
        raise SnapshotError("target_unavailable")
    digest = hashlib.sha256()
    file_count = 0
    total_bytes = 0
    for relative, path in _entries(root_path):
        if path.is_symlink():
            raise SnapshotError("symlink_unsupported")
        relative_bytes = relative.as_posix().encode("utf-8")
        if path.is_dir():
            kind = b"D"
            payload = b""
        elif path.is_file():
            kind = b"F"
            file_count += 1
            if file_count > MAX_FILES:
                raise SnapshotError("target_too_large")
            size = path.stat().st_size
            if size > MAX_FILE_BYTES:
                raise SnapshotError("target_too_large")
            total_bytes += size
            if total_bytes > MAX_TOTAL_BYTES:
                raise SnapshotError("target_too_large")
            payload = path.read_bytes()
        else:
            raise SnapshotError("special_file_unsupported")
        digest.update(kind)
        digest.update(len(relative_bytes).to_bytes(8, "big"))
        digest.update(relative_bytes)
        digest.update(len(payload).to_bytes(8, "big"))
        digest.update(payload)
    return digest.hexdigest()


def copy_target(source: Path, destination: Path) -> None:
    tree_digest(source)
    shutil.copytree(
        source,
        destination,
        symlinks=False,
        ignore=shutil.ignore_patterns(*IGNORED_PARTS),
    )
