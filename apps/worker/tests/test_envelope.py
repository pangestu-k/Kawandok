from __future__ import annotations

import json
import unittest
import uuid

from worker.envelope import EnvelopeError, validate_envelope

MAX_BYTES = 1_048_576


def make_payload(**overrides: object) -> dict:
    payload: dict = {
        "document_id": str(uuid.uuid4()),
        "document_version_id": str(uuid.uuid4()),
        "version_number": 1,
        "storage_bucket": "office-ai-dev",
        "storage_key": "abc-123",
    }
    payload.update(overrides)
    return payload


def make_envelope(**overrides: object) -> dict:
    data: dict = {
        "job_id": str(uuid.uuid4()),
        "job_type": "ingest_document",
        "envelope_version": 1,
        "payload": make_payload(),
    }
    data.update(overrides)
    return data


class ValidateEnvelopeTests(unittest.TestCase):
    def test_valid_envelope_accepted(self) -> None:
        data = make_envelope()
        env = validate_envelope(json.dumps(data), MAX_BYTES)
        self.assertEqual(env.job_id, data["job_id"])
        self.assertEqual(env.job_type, "ingest_document")
        self.assertEqual(env.idempotency_key, data["job_id"])
        self.assertEqual(env.payload["storage_key"], "abc-123")

    def test_valid_envelope_from_bytes(self) -> None:
        raw = json.dumps(make_envelope()).encode("utf-8")
        self.assertEqual(validate_envelope(raw, MAX_BYTES).job_type, "ingest_document")

    def test_invalid_json_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope("{not json", MAX_BYTES)

    def test_non_object_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope("[]", MAX_BYTES)

    def test_unknown_key_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(surprise="x")), MAX_BYTES)

    def test_missing_key_rejected(self) -> None:
        data = make_envelope()
        del data["payload"]
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_disallowed_job_type_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(job_type="run_arbitrary_sql")), MAX_BYTES)

    def test_wrong_envelope_version_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(envelope_version=2)), MAX_BYTES)

    def test_boolean_envelope_version_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(envelope_version=True)), MAX_BYTES)

    def test_non_uuid_job_id_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(job_id="not-a-uuid")), MAX_BYTES)

    def test_non_canonical_uuid_rejected(self) -> None:
        upper = str(uuid.uuid4()).upper()
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(job_id=upper)), MAX_BYTES)

    def test_payload_missing_key_rejected(self) -> None:
        payload = make_payload()
        del payload["storage_key"]
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(payload=payload)), MAX_BYTES)

    def test_payload_bad_version_number_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(version_number=0)), MAX_BYTES)
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(version_number=True)), MAX_BYTES)

    def test_payload_bad_document_uuid_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(document_id="nope")), MAX_BYTES)

    def test_payload_empty_storage_key_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(storage_key="")), MAX_BYTES)

    def test_non_dict_payload_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(make_envelope(payload=[1, 2, 3])), MAX_BYTES)

    def test_oversized_envelope_rejected(self) -> None:
        data = make_envelope(payload=make_payload(blob="x" * (MAX_BYTES + 10)))
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_invalid_utf8_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(b"\xff\xfe\x00garbage", MAX_BYTES)

    def test_explicit_idempotency_key_used(self) -> None:
        key = str(uuid.uuid4())
        env = validate_envelope(json.dumps(make_envelope(idempotency_key=key)), MAX_BYTES)
        self.assertEqual(env.idempotency_key, key)


if __name__ == "__main__":
    unittest.main()
