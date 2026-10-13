from __future__ import annotations

import dataclasses
import os

ALLOWED_ENVS = frozenset({"dev", "prod"})
MAX_ALLOWED_PAYLOAD_BYTES = 16 * 1024 * 1024
DEFAULT_MAX_PAYLOAD_BYTES = 1_048_576


class ConfigError(ValueError):
    pass


@dataclasses.dataclass(frozen=True)
class WorkerConfig:
    env: str
    redis_url: str
    database_url: str
    max_payload_bytes: int

    @classmethod
    def from_env(cls, getenv=os.getenv) -> "WorkerConfig":
        cfg = cls(
            env=getenv("WORKER_ENV", "dev"),
            redis_url=getenv("WORKER_REDIS_URL", "redis://127.0.0.1:6379/0"),
            database_url=getenv("WORKER_DATABASE_URL", ""),
            max_payload_bytes=_int_env(getenv, "WORKER_MAX_PAYLOAD_BYTES", DEFAULT_MAX_PAYLOAD_BYTES),
        )
        cfg.validate()
        return cfg

    def validate(self) -> None:
        if self.env not in ALLOWED_ENVS:
            raise ConfigError(
                f"WORKER_ENV must be one of {sorted(ALLOWED_ENVS)}, got {self.env!r}"
            )
        if not self.redis_url.startswith(("redis://", "rediss://")):
            raise ConfigError(
                "WORKER_REDIS_URL must start with redis:// or rediss://, "
                f"got {self.redis_url!r}"
            )
        if not 0 < self.max_payload_bytes <= MAX_ALLOWED_PAYLOAD_BYTES:
            raise ConfigError(
                f"WORKER_MAX_PAYLOAD_BYTES must be between 1 and {MAX_ALLOWED_PAYLOAD_BYTES}, "
                f"got {self.max_payload_bytes}"
            )


def _int_env(getenv, key: str, default: int) -> int:
    raw = getenv(key, "")
    if raw == "":
        return default
    try:
        return int(raw)
    except ValueError as exc:
        raise ConfigError(f"{key} must be an integer, got {raw!r}") from exc
