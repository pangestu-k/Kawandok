from __future__ import annotations

import logging
import uuid
from dataclasses import dataclass

from worker.chunker import chunk_units
from worker.db import connect
from worker.embedder import Embedder
from worker.envelope import validate_envelope
from worker.finance import MIME_XLSX
from worker.parser import PermanentError, parse_document
from worker.parser import MIME_PDF, MIME_DOCX
from worker.storage import ObjectNotFound, StorageError, StorageFetcher

log = logging.getLogger("worker.ingest")

PARSER_VERSION = "worker-parser-1"
SUPPORTED_MIMES = {MIME_PDF, MIME_DOCX, MIME_XLSX}


class EnvelopeRejected(Exception):
    def __init__(self, raw: bytes, reason: str) -> None:
        super().__init__(reason)
        self.raw = raw
        self.reason = reason


@dataclass(frozen=True)
class ClaimedJob:
    job_id: str
    version_id: str
    job_type: str
    attempts: int
    max_attempts: int


class Processor:
    def __init__(
        self,
        dsn: str,
        fetcher: StorageFetcher,
        embedder: Embedder,
        max_payload_bytes: int,
        chunk_target_chars: int = 1200,
        chunk_overlap_chars: int = 150,
        max_attempts: int = 3,
        backoff_base_seconds: float = 5.0,
    ) -> None:
        self._dsn = dsn
        self._fetcher = fetcher
        self._embedder = embedder
        self._max_payload_bytes = max_payload_bytes
        self._chunk_target_chars = chunk_target_chars
        self._chunk_overlap_chars = chunk_overlap_chars
        self._max_attempts = max_attempts
        self._backoff_base_seconds = backoff_base_seconds

    def handle_message(self, raw: bytes) -> str:
        try:
            envelope = validate_envelope(raw, self._max_payload_bytes)
        except ValueError as exc:
            raise EnvelopeRejected(raw, str(exc)) from exc

        job = self.claim(envelope.job_id)
        if job is None:
            log.info("job %s not claimable (unknown, not due, or already handled)", envelope.job_id)
            return "skipped"
        try:
            self._run(job)
        except PermanentError as exc:
            self._fail(job, exc.code, str(exc))
            return "failed_permanent"
        except Exception as exc:
            log.exception("job %s failed transiently", job.job_id)
            if self._retry_or_fail(job, exc):
                return "failed_permanent"
            return "retry_scheduled"
        return "processed"

    def claim(self, job_id: str) -> ClaimedJob | None:
        try:
            uuid.UUID(job_id)
        except ValueError:
            return None
        with connect(self._dsn) as conn:
            row = conn.execute(
                """
                UPDATE ingestion_jobs
                SET status = 'processing', attempts = attempts + 1, max_attempts = %s,
                    updated_at = now()
                WHERE id = %s::uuid AND status = 'queued' AND run_after <= now()
                RETURNING id::text, document_version_id::text, job_type, attempts, max_attempts
                """,
                (self._max_attempts, job_id),
            ).fetchone()
        if row is None:
            return None
        return ClaimedJob(
            job_id=row[0],
            version_id=row[1],
            job_type=row[2],
            attempts=row[3],
            max_attempts=row[4],
        )

    def process_due(self, limit: int = 10) -> int:
        with connect(self._dsn) as conn:
            rows = conn.execute(
                """
                SELECT id::text FROM ingestion_jobs
                WHERE status = 'queued' AND run_after <= now()
                ORDER BY run_after
                LIMIT %s
                """,
                (limit,),
            ).fetchall()
        handled = 0
        for (job_id,) in rows:
            job = self.claim(job_id)
            if job is None:
                continue
            try:
                self._run(job)
                handled += 1
            except PermanentError as exc:
                self._fail(job, exc.code, str(exc))
            except Exception:
                log.exception("job %s failed transiently", job.job_id)
                self._retry_or_fail(job, None)
        return handled

    def recover_stale(self, stale_after_minutes: int = 10) -> int:
        with connect(self._dsn) as conn:
            cursor = conn.execute(
                """
                UPDATE ingestion_jobs
                SET status = 'queued', updated_at = now()
                WHERE status = 'processing' AND updated_at < now() - make_interval(mins => %s)
                """,
                (stale_after_minutes,),
            )
            return cursor.rowcount or 0

    def _run(self, job: ClaimedJob) -> None:
        with connect(self._dsn) as conn:
            version = conn.execute(
                """
                SELECT mime_type, storage_bucket, storage_key
                FROM document_versions WHERE id = %s::uuid
                """,
                (job.version_id,),
            ).fetchone()
        if version is None:
            raise PermanentError("VERSION_MISSING", f"version {job.version_id} does not exist")

        mime_type, bucket, key = version
        if mime_type not in SUPPORTED_MIMES:
            raise PermanentError("UNSUPPORTED_TYPE", f"no parser for mime type {mime_type!r}")

        try:
            data = self._fetcher.fetch(bucket, key)
        except ObjectNotFound as exc:
            raise PermanentError("OBJECT_MISSING", f"stored object not found: {exc}") from exc
        except StorageError as exc:
            raise RuntimeError(f"storage unavailable: {exc}") from exc

        if mime_type == MIME_XLSX:
            self._run_finance(job, data)
            return

        units = parse_document(mime_type, data)
        chunks = chunk_units(units, self._chunk_target_chars, self._chunk_overlap_chars)
        vectors = self._embedder.embed([chunk.content for chunk in chunks])

        with connect(self._dsn) as conn:
            with conn.transaction():
                conn.execute(
                    "DELETE FROM document_chunks WHERE document_version_id = %s::uuid",
                    (job.version_id,),
                )
                for ordinal, (chunk, vector) in enumerate(zip(chunks, vectors), start=1):
                    conn.execute(
                        """
                        INSERT INTO document_chunks
                            (document_version_id, ordinal, content, page_start, page_end,
                             section_ref, token_count, embedding)
                        VALUES (%s::uuid, %s, %s, %s, %s, %s, %s, %s::vector)
                        """,
                        (
                            job.version_id,
                            ordinal,
                            chunk.content,
                            chunk.page_start,
                            chunk.page_end,
                            chunk.section_ref,
                            chunk.token_count,
                            _vector_literal(vector),
                        ),
                    )
                conn.execute(
                    """
                    UPDATE document_versions
                    SET status = 'ready', embedding_model = %s, parser_version = %s
                    WHERE id = %s::uuid
                    """,
                    (self._embedder.model_id, PARSER_VERSION, job.version_id),
                )
                conn.execute(
                    """
                    UPDATE ingestion_jobs
                    SET status = 'ready', finished_at = now(), last_error_code = NULL,
                        last_error_message = NULL, updated_at = now()
                    WHERE id = %s::uuid
                    """,
                    (job.job_id,),
                )

    def _run_finance(self, job: ClaimedJob, data: bytes) -> None:
        from worker.finance import parse_finance_xlsx

        parsed = parse_finance_xlsx(data)

        with connect(self._dsn) as conn:
            department = conn.execute(
                "SELECT id::text FROM departments WHERE name = %s", (parsed.department,)
            ).fetchone()
        if department is None:
            raise PermanentError(
                "DEPARTMENT_UNKNOWN",
                f"department {parsed.department!r} from the Report sheet does not exist",
            )

        with connect(self._dsn) as conn:
            with conn.transaction():
                conn.execute(
                    """
                    DELETE FROM financial_transactions
                    WHERE report_id IN (
                        SELECT id FROM financial_reports WHERE document_version_id = %s::uuid
                    )
                    """,
                    (job.version_id,),
                )
                conn.execute(
                    "DELETE FROM financial_reports WHERE document_version_id = %s::uuid",
                    (job.version_id,),
                )
                report_id = conn.execute(
                    """
                    INSERT INTO financial_reports
                        (document_version_id, department_id, period_start, period_end,
                         currency, validation_status)
                    VALUES (%s::uuid, %s::uuid, %s, %s, %s, 'validated')
                    RETURNING id::text
                    """,
                    (
                        job.version_id,
                        department[0],
                        parsed.period_start,
                        parsed.period_end,
                        parsed.currency,
                    ),
                ).fetchone()[0]
                for row in parsed.rows:
                    conn.execute(
                        """
                        INSERT INTO financial_transactions
                            (report_id, transaction_date, category, amount_minor_units,
                             currency, source_sheet, source_row, external_transaction_ref)
                        VALUES (%s::uuid, %s, %s, %s, %s, %s, %s, %s)
                        """,
                        (
                            report_id,
                            row.transaction_date,
                            row.category,
                            row.amount_minor_units,
                            row.currency,
                            row.source_sheet,
                            row.source_row,
                            row.external_transaction_ref,
                        ),
                    )
                conn.execute(
                    """
                    UPDATE document_versions
                    SET status = 'ready', parser_version = %s, embedding_model = NULL
                    WHERE id = %s::uuid
                    """,
                    (PARSER_VERSION, job.version_id),
                )
                conn.execute(
                    """
                    UPDATE ingestion_jobs
                    SET status = 'ready', finished_at = now(), last_error_code = NULL,
                        last_error_message = NULL, updated_at = now()
                    WHERE id = %s::uuid
                    """,
                    (job.job_id,),
                )

    def _fail(self, job: ClaimedJob, code: str, message: str) -> None:
        log.error("job %s failed permanently: %s", job.job_id, message)
        with connect(self._dsn) as conn:
            conn.execute(
                """
                UPDATE ingestion_jobs
                SET status = 'failed', last_error_code = %s, last_error_message = %s,
                    finished_at = now(), updated_at = now()
                WHERE id = %s::uuid
                """,
                (code, message[:500], job.job_id),
            )
            conn.execute(
                "UPDATE document_versions SET status = 'failed' WHERE id = %s::uuid",
                (job.version_id,),
            )

    def _retry_or_fail(self, job: ClaimedJob, exc: Exception | None) -> bool:
        """Schedule a retry; returns True when the job failed terminally."""
        message = str(exc) if exc else "unknown error"
        if job.attempts > job.max_attempts:
            self._fail(job, "PROCESSING_FAILED", message)
            return True
        delay = self._backoff_base_seconds * (2 ** max(0, job.attempts - 1))
        with connect(self._dsn) as conn:
            conn.execute(
                """
                UPDATE ingestion_jobs
                SET status = 'queued', run_after = now() + make_interval(secs => %s),
                    last_error_code = 'PROCESSING_FAILED', last_error_message = %s, updated_at = now()
                WHERE id = %s::uuid
                """,
                (min(delay, 3600.0), message[:500], job.job_id),
            )
            conn.execute(
                "UPDATE document_versions SET status = 'queued' WHERE id = %s::uuid AND status = 'processing'",
                (job.version_id,),
            )
        return False


def _vector_literal(vector: list[float]) -> str:
    return "[" + ",".join(f"{x:.7f}" for x in vector) + "]"
