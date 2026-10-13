from __future__ import annotations

import json
import uuid
from dataclasses import dataclass
from typing import Any

ENVELOPE_VERSION = 1
ALLOWED_JOB_TYPES = frozenset({"ingest_document", "reindex_document"})
ALLOWED_KEYS = frozenset({"job_id", "job_type", "envelope_version", "idempotency_key", "payload"})
REQUIRED_KEYS = frozenset({"job_id", "job_type", "envelope_version", "payload"})


class EnvelopeError(ValueError):
    pass


@dataclass(frozen=True)
class JobEnvelope:
    job_id: str
    job_type: str
    envelope_version: int
    idempotency_key: str
    payload: dict[str, Any]


def validate_envelope(raw: bytes | str, max_payload_bytes: int) -> JobEnvelope:
    if isinstance(raw, bytes):
        try:
            raw = raw.decode("utf-8")
        except UnicodeDecodeError as exc:
            raise EnvelopeError("envelope must be UTF-8") from exc

    if len(raw.encode("utf-8")) > max_payload_bytes:
        raise EnvelopeError("envelope exceeds maximum size")

    try:
        data = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise EnvelopeError("envelope is not valid JSON") from exc

    if not isinstance(data, dict):
        raise EnvelopeError("envelope must be a JSON object")

    unknown = set(data) - ALLOWED_KEYS
    if unknown:
        raise EnvelopeError(f"unknown keys: {sorted(unknown)}")

    missing = REQUIRED_KEYS - set(data)
    if missing:
        raise EnvelopeError(f"missing keys: {sorted(missing)}")

    _validate_uuid(data["job_id"], "job_id")

    job_type = data["job_type"]
    if job_type not in ALLOWED_JOB_TYPES:
        raise EnvelopeError(f"job_type {job_type!r} is not allowed")

    version = data["envelope_version"]
    if isinstance(version, bool) or not isinstance(version, int) or version != ENVELOPE_VERSION:
        raise EnvelopeError(f"envelope_version must be {ENVELOPE_VERSION}")

    payload = data["payload"]
    if not isinstance(payload, dict):
        raise EnvelopeError("payload must be a JSON object")

    idempotency_key = data.get("idempotency_key", data["job_id"])
    _validate_uuid(idempotency_key, "idempotency_key")

    payload_size = len(json.dumps(payload, separators=(",", ":")).encode("utf-8"))
    if payload_size > max_payload_bytes:
        raise EnvelopeError("payload exceeds maximum size")

    return JobEnvelope(
        job_id=data["job_id"],
        job_type=job_type,
        envelope_version=ENVELOPE_VERSION,
        idempotency_key=idempotency_key,
        payload=payload,
    )


def _validate_uuid(value: object, field: str) -> None:
    if not isinstance(value, str):
        raise EnvelopeError(f"{field} must be a string UUID")
    try:
        parsed = uuid.UUID(value)
    except (ValueError, AttributeError) as exc:
        raise EnvelopeError(f"{field} is not a valid UUID: {value!r}") from exc
    if str(parsed) != value:
        raise EnvelopeError(f"{field} must be a canonical hyphenated lowercase UUID")
