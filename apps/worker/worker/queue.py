from __future__ import annotations

import logging
import threading
from typing import Callable

import redis

from worker.ingest import EnvelopeRejected

log = logging.getLogger("worker.queue")


class Consumer:
    def __init__(
        self,
        client: redis.Redis,
        stream: str,
        group: str,
        consumer_name: str,
        handle_message: Callable[[bytes], str],
        block_ms: int = 2000,
        batch: int = 10,
    ) -> None:
        self._client = client
        self._stream = stream
        self._group = group
        self._consumer = consumer_name
        self._handle_message = handle_message
        self._block_ms = block_ms
        self._batch = batch

    def ensure_group(self) -> None:
        try:
            self._client.xgroup_create(self._stream, self._group, id="0-0", mkstream=True)
        except redis.ResponseError as exc:
            if "BUSYGROUP" not in str(exc):
                raise

    def process_batch(self) -> int:
        results = self._client.xreadgroup(
            self._group,
            self._consumer,
            {self._stream: ">"},
            count=self._batch,
            block=self._block_ms,
        )
        handled = 0
        for _, entries in results or []:
            for entry_id, fields in entries:
                self._handle_entry(entry_id, fields)
                handled += 1
        return handled

    def _handle_entry(self, entry_id, fields: dict) -> None:
        raw = fields.get("envelope")
        if isinstance(raw, str):
            raw = raw.encode("utf-8")
        try:
            if raw is None:
                raise EnvelopeRejected(b"", "stream entry is missing the envelope field")
            self._handle_message(raw)
        except EnvelopeRejected as exc:
            log.warning("malformed envelope %s: %s", entry_id, exc.reason)
            self._dead_letter(exc.raw, exc.reason)
        except Exception:
            log.exception("unexpected failure handling %s; acking to avoid redelivery loop", entry_id)
            self._dead_letter(raw or b"", "unexpected worker error")
        self._ack(entry_id)

    def _dead_letter(self, raw: bytes, reason: str) -> None:
        try:
            self._client.xadd(
                f"{self._stream}:dead",
                {"reason": reason, "envelope": raw.decode("utf-8", "replace")},
            )
        except Exception:
            log.exception("failed to write dead-letter entry")

    def _ack(self, entry_id) -> None:
        try:
            self._client.xack(self._stream, self._group, entry_id)
        except Exception:
            log.exception("failed to ack %s", entry_id)

    def run_forever(
        self,
        stop_event: threading.Event,
        process_due: Callable[[], int],
        poll_interval_seconds: float = 5.0,
    ) -> None:
        while not stop_event.is_set():
            try:
                self.process_batch()
                process_due()
            except Exception:
                log.exception("consumer loop error; retrying after interval")
                stop_event.wait(poll_interval_seconds)
