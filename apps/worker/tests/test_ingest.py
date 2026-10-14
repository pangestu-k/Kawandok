from __future__ import annotations

import os
import unittest
import uuid

from worker.embedder import MockEmbedder
from worker.ingest import EnvelopeRejected, Processor
from worker.parser import MIME_DOCX
from worker.storage import MemoryFetcher

try:
    import psycopg  # noqa: F401

    HAVE_PSYCOPG = True
except ImportError:
    HAVE_PSYCOPG = False


def make_docx_bytes(paragraphs: list[str]) -> bytes:
    import io

    import docx

    document = docx.Document()
    for text in paragraphs:
        document.add_paragraph(text)
    buffer = io.BytesIO()
    document.save(buffer)
    return buffer.getvalue()


def make_envelope_bytes(job_id: str, version_id: str, document_id: str) -> bytes:
    import json

    return json.dumps(
        {
            "job_id": job_id,
            "job_type": "ingest_document",
            "envelope_version": 1,
            "idempotency_key": job_id,
            "payload": {
                "document_id": document_id,
                "document_version_id": version_id,
                "version_number": 1,
                "storage_bucket": "test-bucket",
                "storage_key": "objects/abc",
            },
        }
    ).encode("utf-8")


@unittest.skipUnless(HAVE_PSYCOPG, "psycopg not installed")
@unittest.skipUnless(os.getenv("WORKER_TEST_DATABASE_URL"), "WORKER_TEST_DATABASE_URL not set")
class ProcessorTests(unittest.TestCase):
    def setUp(self) -> None:
        import psycopg

        self.dsn = os.environ["WORKER_TEST_DATABASE_URL"]
        self.conn = psycopg.connect(self.dsn, autocommit=True)
        self.conn.execute(
            "TRUNCATE document_chunks, ingestion_outbox, ingestion_jobs, "
            "document_versions, documents, document_grants, audit_events, "
            "user_roles, users, roles, departments CASCADE"
        )
        self.conn.execute("INSERT INTO departments (name) VALUES ('Finance')")
        self.conn.execute(
            """
            INSERT INTO users (id, email, display_name, department_id)
            VALUES (gen_random_uuid(), 'worker-test@dev.local', 'Worker Test',
                    (SELECT id FROM departments WHERE name = 'Finance'))
            """
        )
        self.conn.execute(
            """
            INSERT INTO documents (title, classification, owning_department_id, created_by)
            VALUES ('Test doc', 'internal',
                    (SELECT id FROM departments WHERE name = 'Finance'),
                    (SELECT id FROM users WHERE email = 'worker-test@dev.local'))
            """
        )
        self.conn.execute(
            """
            INSERT INTO document_versions
                (document_id, version_number, original_filename, mime_type, size_bytes,
                 sha256, storage_bucket, storage_key, status)
            VALUES
                ((SELECT id FROM documents ORDER BY created_at LIMIT 1), 1, 'notes.docx',
                 %s, 100, 'deadbeef', 'test-bucket', 'objects/abc', 'queued')
            """,
            (MIME_DOCX,),
        )
        self.conn.execute(
            """
            INSERT INTO ingestion_jobs (document_version_id, job_type, status)
            SELECT id, 'ingest', 'queued' FROM document_versions ORDER BY created_at LIMIT 1
            """
        )
        self.version_id = self.conn.execute(
            "SELECT id::text FROM document_versions ORDER BY created_at LIMIT 1"
        ).fetchone()[0]
        self.document_id = self.conn.execute(
            "SELECT id::text FROM documents ORDER BY created_at LIMIT 1"
        ).fetchone()[0]
        self.job_id = self.conn.execute(
            "SELECT id::text FROM ingestion_jobs ORDER BY created_at LIMIT 1"
        ).fetchone()[0]

    def tearDown(self) -> None:
        self.conn.execute(
            "TRUNCATE document_chunks, ingestion_outbox, ingestion_jobs, "
            "document_versions, documents, document_grants, audit_events, "
            "user_roles, users, roles, departments CASCADE"
        )
        self.conn.close()

    def new_processor(self, fetcher: MemoryFetcher) -> Processor:
        return Processor(
            dsn=self.dsn,
            fetcher=fetcher,
            embedder=MockEmbedder(1536),
            max_payload_bytes=1_048_576,
            chunk_target_chars=400,
            chunk_overlap_chars=40,
            max_attempts=2,
            backoff_base_seconds=0.0,
        )

    def fetcher_with_content(self, content: bytes) -> MemoryFetcher:
        fetcher = MemoryFetcher()
        fetcher.put("test-bucket", "objects/abc", content)
        return fetcher

    def test_successful_ingestion_marks_ready_and_writes_chunks(self) -> None:
        fetcher = self.fetcher_with_content(
            make_docx_bytes(["Alpha paragraph about policy.", "Beta paragraph about totals."])
        )
        processor = self.new_processor(fetcher)
        outcome = processor.handle_message(
            make_envelope_bytes(self.job_id, self.version_id, self.document_id)
        )
        self.assertEqual(outcome, "processed")

        row = self.conn.execute(
            "SELECT status FROM document_versions WHERE id = %s::uuid", (self.version_id,)
        ).fetchone()
        self.assertEqual(row[0], "ready")

        chunk_count, token_sum = self.conn.execute(
            "SELECT count(*), sum(token_count) FROM document_chunks WHERE document_version_id = %s::uuid",
            (self.version_id,),
        ).fetchone()
        self.assertGreaterEqual(chunk_count, 1)
        self.assertGreater(token_sum, 0)

        job = self.conn.execute(
            "SELECT status, attempts FROM ingestion_jobs WHERE id = %s::uuid", (self.job_id,)
        ).fetchone()
        self.assertEqual(job[0], "ready")
        self.assertEqual(job[1], 1)

        model = self.conn.execute(
            "SELECT embedding_model, parser_version FROM document_versions WHERE id = %s::uuid",
            (self.version_id,),
        ).fetchone()
        self.assertEqual(model[0], "mock-embedding-v1")
        self.assertEqual(model[1], "worker-parser-1")

    def test_reprocessing_does_not_duplicate_chunks(self) -> None:
        fetcher = self.fetcher_with_content(make_docx_bytes(["Only one paragraph."]))
        processor = self.new_processor(fetcher)
        envelope = make_envelope_bytes(self.job_id, self.version_id, self.document_id)

        self.assertEqual(processor.handle_message(envelope), "processed")
        count_first = self.conn.execute(
            "SELECT count(*) FROM document_chunks WHERE document_version_id = %s::uuid",
            (self.version_id,),
        ).fetchone()[0]

        job_id = self.conn.execute(
            "UPDATE ingestion_jobs SET status = 'queued', run_after = now() - interval '1 minute' "
            "WHERE id = %s::uuid RETURNING id::text",
            (self.job_id,),
        ).fetchone()[0]
        self.assertEqual(processor.handle_message(envelope), "processed")
        self.assertEqual(job_id, self.job_id)

        count_second = self.conn.execute(
            "SELECT count(*) FROM document_chunks WHERE document_version_id = %s::uuid",
            (self.version_id,),
        ).fetchone()[0]
        self.assertEqual(count_first, count_second)

    def test_missing_object_fails_permanently(self) -> None:
        processor = self.new_processor(MemoryFetcher())
        outcome = processor.handle_message(
            make_envelope_bytes(self.job_id, self.version_id, self.document_id)
        )
        self.assertEqual(outcome, "failed_permanent")
        job = self.conn.execute(
            "SELECT status, last_error_code FROM ingestion_jobs WHERE id = %s::uuid",
            (self.job_id,),
        ).fetchone()
        self.assertEqual(job[0], "failed")
        self.assertEqual(job[1], "OBJECT_MISSING")
        version = self.conn.execute(
            "SELECT status FROM document_versions WHERE id = %s::uuid", (self.version_id,)
        ).fetchone()
        self.assertEqual(version[0], "failed")

    def test_transient_failure_schedules_retry_then_terminal(self) -> None:
        class FlakyFetcher(MemoryFetcher):
            def __init__(self) -> None:
                super().__init__()
                self.calls = 0

            def fetch(self, bucket: str, key: str) -> bytes:
                self.calls += 1
                raise TimeoutError("simulated storage timeout")

        fetcher = FlakyFetcher()
        processor = self.new_processor(fetcher)
        envelope = make_envelope_bytes(self.job_id, self.version_id, self.document_id)

        self.assertEqual(processor.handle_message(envelope), "retry_scheduled")
        state = self.conn.execute(
            "SELECT status, attempts FROM ingestion_jobs WHERE id = %s::uuid", (self.job_id,)
        ).fetchone()
        self.assertEqual(state[0], "queued")
        self.assertEqual(state[1], 1)

        self.conn.execute(
            "UPDATE ingestion_jobs SET run_after = now() - interval '1 minute' WHERE id = %s::uuid",
            (self.job_id,),
        )
        self.assertEqual(processor.handle_message(envelope), "retry_scheduled")
        state = self.conn.execute(
            "SELECT status, attempts FROM ingestion_jobs WHERE id = %s::uuid", (self.job_id,)
        ).fetchone()
        self.assertEqual(state[0], "queued")
        self.assertEqual(state[1], 2)

        self.conn.execute(
            "UPDATE ingestion_jobs SET run_after = now() - interval '1 minute' WHERE id = %s::uuid",
            (self.job_id,),
        )
        self.assertEqual(processor.handle_message(envelope), "failed_permanent")
        state = self.conn.execute(
            "SELECT status, attempts, last_error_code FROM ingestion_jobs WHERE id = %s::uuid",
            (self.job_id,),
        ).fetchone()
        self.assertEqual(state[0], "failed")
        self.assertEqual(state[1], 3)
        self.assertEqual(state[2], "PROCESSING_FAILED")

    def test_claim_is_one_shot(self) -> None:
        fetcher = self.fetcher_with_content(make_docx_bytes(["Claim once."]))
        processor = self.new_processor(fetcher)
        first = processor.claim(self.job_id)
        self.assertIsNotNone(first)
        second = processor.claim(self.job_id)
        self.assertIsNone(second)

    def test_malformed_envelope_is_rejected(self) -> None:
        processor = self.new_processor(MemoryFetcher())
        with self.assertRaises(EnvelopeRejected):
            processor.handle_message(b"this is not json")

    def test_unknown_job_is_skipped(self) -> None:
        fetcher = self.fetcher_with_content(make_docx_bytes(["Nobody claims this."]))
        processor = self.new_processor(fetcher)
        outcome = processor.handle_message(
            make_envelope_bytes(str(uuid.uuid4()), self.version_id, self.document_id)
        )
        self.assertEqual(outcome, "skipped")


if __name__ == "__main__":
    unittest.main()
