from __future__ import annotations

import json
import unittest
import uuid

from worker.envelope import EnvelopeError, validate_envelope

MAX_BYTES = 1_048_576


def make_envelope(**overrides: object) -> dict:
    data: dict = {
        "job_id": str(uuid.uuid4()),
        "job_type": "ingest_document",
        "envelope_version": 1,
        "payload": {"document_version_id": "dv_123"},
    }
    data.update(overrides)
    return data


class ValidateEnvelopeTests(unittest.TestCase):
    def test_valid_envelope_accepted(self) -> None:
        data = make_envelope()
        env = validate_envelope(json.dumps(data), MAX_BYTES)
        self.assertEqual(env.job_id, data["job_id"])
        self.assertEqual(env.job_type, "ingest_document")
        self.assertEqual(env.envelope_version, 1)
        self.assertEqual(env.idempotency_key, data["job_id"])
        self.assertEqual(env.payload, {"document_version_id": "dv_123"})

    def test_valid_envelope_from_bytes(self) -> None:
        raw = json.dumps(make_envelope()).encode("utf-8")
        env = validate_envelope(raw, MAX_BYTES)
        self.assertEqual(env.job_type, "ingest_document")

    def test_explicit_idempotency_key_used(self) -> None:
        key = str(uuid.uuid4())
        env = validate_envelope(json.dumps(make_envelope(idempotency_key=key)), MAX_BYTES)
        self.assertEqual(env.idempotency_key, key)

    def test_invalid_json_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope("{not json", MAX_BYTES)

    def test_non_object_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope("[]", MAX_BYTES)

    def test_unknown_key_rejected(self) -> None:
        data = make_envelope(surprise="x")
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_missing_key_rejected(self) -> None:
        data = make_envelope()
        del data["payload"]
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_disallowed_job_type_rejected(self) -> None:
        data = make_envelope(job_type="run_arbitrary_sql")
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_wrong_envelope_version_rejected(self) -> None:
        data = make_envelope(envelope_version=2)
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_boolean_envelope_version_rejected(self) -> None:
        data = make_envelope(envelope_version=True)
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_non_uuid_job_id_rejected(self) -> None:
        data = make_envelope(job_id="not-a-uuid")
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_non_canonical_uuid_rejected(self) -> None:
        upper = str(uuid.uuid4()).upper()
        data = make_envelope(job_id=upper)
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_non_dict_payload_rejected(self) -> None:
        data = make_envelope(payload=[1, 2, 3])
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_oversized_envelope_rejected(self) -> None:
        data = make_envelope(payload={"blob": "x" * (MAX_BYTES + 10)})
        with self.assertRaises(EnvelopeError):
            validate_envelope(json.dumps(data), MAX_BYTES)

    def test_invalid_utf8_rejected(self) -> None:
        with self.assertRaises(EnvelopeError):
            validate_envelope(b"\xff\xfe\x00garbage", MAX_BYTES)


if __name__ == "__main__":
    unittest.main()
