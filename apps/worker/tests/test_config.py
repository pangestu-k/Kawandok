from __future__ import annotations

import unittest

from worker.config import ConfigError, WorkerConfig


class WorkerConfigTests(unittest.TestCase):
    def test_defaults_are_applied(self) -> None:
        cfg = WorkerConfig.from_env(lambda key, default="": default)
        self.assertEqual(cfg.env, "dev")
        self.assertEqual(cfg.redis_url, "redis://127.0.0.1:6379/0")
        self.assertEqual(cfg.database_url, "")
        self.assertEqual(cfg.max_payload_bytes, 1_048_576)

    def test_explicit_values_are_kept(self) -> None:
        env = {
            "WORKER_ENV": "prod",
            "WORKER_REDIS_URL": "rediss://redis.example:6379/0",
            "WORKER_DATABASE_URL": "postgresql://worker@db/office_ai",
            "WORKER_MAX_PAYLOAD_BYTES": "2048",
        }
        cfg = WorkerConfig.from_env(lambda key, default="": env.get(key, default))
        self.assertEqual(cfg.env, "prod")
        self.assertEqual(cfg.redis_url, "rediss://redis.example:6379/0")
        self.assertEqual(cfg.database_url, "postgresql://worker@db/office_ai")
        self.assertEqual(cfg.max_payload_bytes, 2048)

    def test_invalid_env_rejected(self) -> None:
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(
                lambda key, default="": "staging" if key == "WORKER_ENV" else default
            )

    def test_invalid_redis_scheme_rejected(self) -> None:
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(
                lambda key, default="": (
                    "http://127.0.0.1:6379" if key == "WORKER_REDIS_URL" else default
                )
            )

    def test_zero_payload_limit_rejected(self) -> None:
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(
                lambda key, default="": "0" if key == "WORKER_MAX_PAYLOAD_BYTES" else default
            )

    def test_oversized_payload_limit_rejected(self) -> None:
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(
                lambda key, default="": (
                    str(17 * 1024 * 1024) if key == "WORKER_MAX_PAYLOAD_BYTES" else default
                )
            )

    def test_non_integer_payload_limit_rejected(self) -> None:
        with self.assertRaises(ConfigError):
            WorkerConfig.from_env(
                lambda key, default="": (
                    "big" if key == "WORKER_MAX_PAYLOAD_BYTES" else default
                )
            )


if __name__ == "__main__":
    unittest.main()
