from __future__ import annotations

import unittest

from worker.config import ConfigError, WorkerConfig


def base_env() -> dict:
    return {
        "WORKER_ENV": "dev",
        "WORKER_DATABASE_URL": "postgresql://worker@127.0.0.1:15432/office_ai",
    }


def env_getter(env: dict):
    return lambda key, default="": env.get(key, default)


class WorkerConfigTests(unittest.TestCase):
    def test_defaults_are_applied(self) -> None:
        cfg = WorkerConfig.from_env(env_getter(base_env()))
        self.assertEqual(cfg.env, "dev")
        self.assertEqual(cfg.redis_url, "redis://127.0.0.1:16379/0")
        self.assertEqual(cfg.max_payload_bytes, 1_048_576)
        self.assertEqual(cfg.stream, "office-ai:ingest:v1")
        self.assertEqual(cfg.consumer_group, "workers")
        self.assertTrue(cfg.consumer_name)
        self.assertEqual(cfg.embedding_dim, 1536)
        self.assertEqual(cfg.chunk_target_chars, 1200)
        self.assertEqual(cfg.chunk_overlap_chars, 150)
        self.assertEqual(cfg.poll_interval_seconds, 5.0)
        self.assertEqual(cfg.max_attempts, 3)
        self.assertEqual(cfg.storage_backend, "memory")

    def test_explicit_values_are_kept(self) -> None:
        env = base_env()
        env.update(
            {
                "WORKER_ENV": "prod",
                "WORKER_REDIS_URL": "rediss://redis.example:6379/0",
                "WORKER_EMBEDDING_DIM": "768",
                "WORKER_STORAGE_BACKEND": "s3",
                "WORKER_STORAGE_ENDPOINT": "https://acct.r2.cloudflarestorage.com",
                "WORKER_STORAGE_BUCKET": "docs",
                "WORKER_STORAGE_ACCESS_KEY": "key",
                "WORKER_STORAGE_SECRET_KEY": "secret",
            }
        )
        cfg = WorkerConfig.from_env(env_getter(env))
        self.assertEqual(cfg.env, "prod")
        self.assertEqual(cfg.redis_url, "rediss://redis.example:6379/0")
        self.assertEqual(cfg.embedding_dim, 768)
        self.assertEqual(cfg.storage_backend, "s3")
        self.assertEqual(cfg.storage_bucket, "docs")

    def test_database_url_is_required(self) -> None:
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(env_getter({}))

    def test_invalid_env_rejected(self) -> None:
        env = base_env()
        env["WORKER_ENV"] = "staging"
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(env_getter(env))

    def test_invalid_redis_scheme_rejected(self) -> None:
        env = base_env()
        env["WORKER_REDIS_URL"] = "http://127.0.0.1:16379"
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(env_getter(env))

    def test_s3_backend_requires_full_config(self) -> None:
        env = base_env()
        env["WORKER_STORAGE_BACKEND"] = "s3"
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(env_getter(env))

    def test_overlap_must_be_smaller_than_target(self) -> None:
        env = base_env()
        env["WORKER_CHUNK_TARGET_CHARS"] = "200"
        env["WORKER_CHUNK_OVERLAP_CHARS"] = "200"
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(env_getter(env))

    def test_max_attempts_bounds(self) -> None:
        env = base_env()
        env["WORKER_MAX_ATTEMPTS"] = "11"
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(env_getter(env))

    def test_non_integer_payload_limit_rejected(self) -> None:
        env = base_env()
        env["WORKER_MAX_PAYLOAD_BYTES"] = "big"
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(env_getter(env))


if __name__ == "__main__":
    unittest.main()
