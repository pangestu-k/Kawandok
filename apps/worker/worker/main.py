from __future__ import annotations

import logging
import signal
import sys
import threading

import redis

from worker.config import WorkerConfig
from worker.embedder import MockEmbedder, OpenAIEmbedder
from worker.ingest import Processor
from worker.queue import Consumer
from worker.storage import build_fetcher


def main() -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    log = logging.getLogger("worker")
    try:
        cfg = WorkerConfig.from_env()
    except ValueError as exc:
        log.error("invalid configuration: %s", exc)
        return 1

    processor = Processor(
        dsn=cfg.database_url,
        fetcher=build_fetcher(cfg),
        embedder=build_embedder(cfg),
        max_payload_bytes=cfg.max_payload_bytes,
        chunk_target_chars=cfg.chunk_target_chars,
        chunk_overlap_chars=cfg.chunk_overlap_chars,
        max_attempts=cfg.max_attempts,
        backoff_base_seconds=cfg.backoff_base_seconds,
    )

    recovered = processor.recover_stale()
    if recovered:
        log.info("recovered %d stale processing job(s) back to queued", recovered)

    client = redis.Redis.from_url(cfg.redis_url, decode_responses=True)
    consumer = Consumer(
        client=client,
        stream=cfg.stream,
        group=cfg.consumer_group,
        consumer_name=cfg.consumer_name,
        handle_message=processor.handle_message,
    )
    consumer.ensure_group()
    log.info(
        "worker ready (stream=%s group=%s consumer=%s backend=%s)",
        cfg.stream,
        cfg.consumer_group,
        cfg.consumer_name,
        cfg.storage_backend,
    )

    stop_event = threading.Event()

    def request_stop(signum, frame):
        log.info("signal %s received; shutting down", signum)
        stop_event.set()

    signal.signal(signal.SIGINT, request_stop)
    signal.signal(signal.SIGTERM, request_stop)

    consumer.run_forever(
        stop_event,
        lambda: processor.process_due(),
        poll_interval_seconds=cfg.poll_interval_seconds,
    )
    log.info("worker stopped")
    return 0


def build_embedder(cfg: WorkerConfig):
    if cfg.embedding_provider == "openai":
        return OpenAIEmbedder(cfg.embedding_base_url, cfg.embedding_api_key, cfg.embedding_model, cfg.embedding_dim)
    return MockEmbedder(cfg.embedding_dim)


if __name__ == "__main__":
    sys.exit(main())
