from __future__ import annotations

import os
from pathlib import Path

_MAX_LEVELS = 8


def find_dotenv(start_dir: str | None = None) -> str | None:
    current = Path(start_dir or os.getcwd()).resolve()
    for _ in range(_MAX_LEVELS):
        candidate = current / ".env"
        if candidate.is_file():
            return str(candidate)
        if current.parent == current:
            return None
        current = current.parent
    return None


def load_dotenv(start_dir: str | None = None) -> str | None:
    """Load KEY=VALUE pairs from the nearest .env without overriding
    existing environment variables. Returns the path that was loaded."""
    path = find_dotenv(start_dir)
    if path is None:
        return None
    with open(path, "r", encoding="utf-8") as handle:
        for raw in handle:
            line = raw.strip().rstrip("\r")
            if not line or line.startswith("#"):
                continue
            if line.startswith("export "):
                line = line[len("export "):]
            if "=" not in line:
                continue
            key, _, value = line.partition("=")
            key = key.strip()
            value = value.strip()
            if len(value) >= 2 and value[0] == value[-1] and value[0] in ("'", '"'):
                value = value[1:-1]
            if key and " " not in key and key not in os.environ:
                os.environ[key] = value
    return path
