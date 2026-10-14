from __future__ import annotations

import dataclasses
import os
import socket

ALLOWED_ENVS = frozenset({"dev", "prod"})
ALLOWED_STORAGE_BACKENDS = frozenset({"memory", "s3"})
ALLOWED_EMBEDDING_PROVIDERS = frozenset({"mock", "openai"})
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
    stream: str
    consumer_group: str
    consumer_name: str
    embedding_dim: int
    embedding_provider: str
    embedding_base_url: str
    embedding_model: str
    embedding_api_key: str
    chunk_target_chars: int
    chunk_overlap_chars: int
    poll_interval_seconds: float
    max_attempts: int
    backoff_base_seconds: float
    storage_backend: str
    storage_endpoint: str
    storage_bucket: str
    storage_access_key: str
    storage_secret_key: str

    @classmethod
    def from_env(cls, getenv=os.getenv) -> "WorkerConfig":
        cfg = cls(
            env=getenv("WORKER_ENV", "dev"),
            redis_url=getenv("WORKER_REDIS_URL", "redis://127.0.0.1:16379/0"),
            database_url=getenv("WORKER_DATABASE_URL", ""),
            max_payload_bytes=_int_env(getenv, "WORKER_MAX_PAYLOAD_BYTES", DEFAULT_MAX_PAYLOAD_BYTES),
            stream=getenv("WORKER_STREAM", "office-ai:ingest:v1"),
            consumer_group=getenv("WORKER_CONSUMER_GROUP", "workers"),
            consumer_name=getenv("WORKER_CONSUMER_NAME", _default_consumer_name()),
            embedding_dim=_int_env(getenv, "WORKER_EMBEDDING_DIM", 1536),
            embedding_provider=getenv("WORKER_EMBEDDING_PROVIDER", "mock").strip().lower(),
            embedding_base_url=getenv("WORKER_EMBEDDING_BASE_URL", "").strip(),
            embedding_model=getenv("WORKER_EMBEDDING_MODEL", "").strip(),
            embedding_api_key=getenv("WORKER_EMBEDDING_API_KEY", "").strip(),
            chunk_target_chars=_int_env(getenv, "WORKER_CHUNK_TARGET_CHARS", 1200),
            chunk_overlap_chars=_int_env(getenv, "WORKER_CHUNK_OVERLAP_CHARS", 150),
            poll_interval_seconds=_float_env(getenv, "WORKER_POLL_INTERVAL_SECONDS", 5.0),
            max_attempts=_int_env(getenv, "WORKER_MAX_ATTEMPTS", 3),
            backoff_base_seconds=_float_env(getenv, "WORKER_BACKOFF_BASE_SECONDS", 5.0),
            storage_backend=getenv("WORKER_STORAGE_BACKEND", "memory"),
            storage_endpoint=getenv("WORKER_STORAGE_ENDPOINT", ""),
            storage_bucket=getenv("WORKER_STORAGE_BUCKET", ""),
            storage_access_key=getenv("WORKER_STORAGE_ACCESS_KEY", ""),
            storage_secret_key=getenv("WORKER_STORAGE_SECRET_KEY", ""),
        )
        cfg.validate()
        return cfg

    def validate(self) -> None:
        if self.env not in ALLOWED_ENVS:
            raise ConfigError(
                f"WORKER_ENV must be one of {sorted(ALLOWED_ENVS)}, got {self.env!r}"
            )
        if not self.database_url:
            raise ConfigError("WORKER_DATABASE_URL is required")
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
        if not self.stream or len(self.stream) > 256:
            raise ConfigError("WORKER_STREAM must be a non-empty string of at most 256 characters")
        if not self.consumer_group or not self.consumer_name:
            raise ConfigError("WORKER_CONSUMER_GROUP and WORKER_CONSUMER_NAME must be non-empty")
        if not 1 <= self.embedding_dim <= 10_000:
            raise ConfigError(f"WORKER_EMBEDDING_DIM must be between 1 and 10000, got {self.embedding_dim}")
        if self.embedding_provider not in ALLOWED_EMBEDDING_PROVIDERS:
            raise ConfigError(f"WORKER_EMBEDDING_PROVIDER must be one of {sorted(ALLOWED_EMBEDDING_PROVIDERS)}")
        if self.embedding_provider == "openai" and not all(
            (self.embedding_base_url, self.embedding_model, self.embedding_api_key)
        ):
            raise ConfigError("WORKER_EMBEDDING_BASE_URL, WORKER_EMBEDDING_MODEL, and WORKER_EMBEDDING_API_KEY are required for openai")
        if not 100 <= self.chunk_target_chars <= 100_000:
            raise ConfigError(
                f"WORKER_CHUNK_TARGET_CHARS must be between 100 and 100000, got {self.chunk_target_chars}"
            )
        if not 0 <= self.chunk_overlap_chars < self.chunk_target_chars:
            raise ConfigError(
                "WORKER_CHUNK_OVERLAP_CHARS must be smaller than WORKER_CHUNK_TARGET_CHARS, "
                f"got {self.chunk_overlap_chars} vs {self.chunk_target_chars}"
            )
        if not 0 < self.poll_interval_seconds <= 300:
            raise ConfigError(
                f"WORKER_POLL_INTERVAL_SECONDS must be between 0 and 300, got {self.poll_interval_seconds}"
            )
        if not 1 <= self.max_attempts <= 10:
            raise ConfigError(f"WORKER_MAX_ATTEMPTS must be between 1 and 10, got {self.max_attempts}")
        if not 0 <= self.backoff_base_seconds <= 3600:
            raise ConfigError(
                f"WORKER_BACKOFF_BASE_SECONDS must be between 0 and 3600, got {self.backoff_base_seconds}"
            )
        if self.storage_backend not in ALLOWED_STORAGE_BACKENDS:
            raise ConfigError(
                f"WORKER_STORAGE_BACKEND must be one of {sorted(ALLOWED_STORAGE_BACKENDS)}, "
                f"got {self.storage_backend!r}"
            )
        if self.storage_backend == "s3":
            for name in (
                "WORKER_STORAGE_ENDPOINT",
                "WORKER_STORAGE_BUCKET",
                "WORKER_STORAGE_ACCESS_KEY",
                "WORKER_STORAGE_SECRET_KEY",
            ):
                if not getattr(self, _config_field(name)):
                    raise ConfigError(f"{name} is required when the storage backend is 's3'")


def _config_field(env_name: str) -> str:
    return {
        "WORKER_STORAGE_ENDPOINT": "storage_endpoint",
        "WORKER_STORAGE_BUCKET": "storage_bucket",
        "WORKER_STORAGE_ACCESS_KEY": "storage_access_key",
        "WORKER_STORAGE_SECRET_KEY": "storage_secret_key",
    }[env_name]


def _default_consumer_name() -> str:
    try:
        host = socket.gethostname()
    except OSError:
        host = "unknown"
    return f"{host}:{os.getpid()}"


def _int_env(getenv, key: str, default: int) -> int:
    raw = getenv(key, "")
    if raw == "":
        return default
    try:
        return int(raw)
    except ValueError as exc:
        raise ConfigError(f"{key} must be an integer, got {raw!r}") from exc


def _float_env(getenv, key: str, default: float) -> float:
    raw = getenv(key, "")
    if raw == "":
        return default
    try:
        return float(raw)
    except ValueError as exc:
        raise ConfigError(f"{key} must be a number, got {raw!r}") from exc
