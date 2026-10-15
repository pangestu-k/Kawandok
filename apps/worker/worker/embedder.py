from __future__ import annotations

import hashlib
import json
import math
import urllib.error
import urllib.request
from typing import Protocol

MODEL_ID = "mock-embedding-v1"
MAX_DIM = 4096


class Embedder(Protocol):
    @property
    def model_id(self) -> str: ...

    def embed(self, texts: list[str]) -> list[list[float]]: ...


class MockEmbedder:
    def __init__(self, dim: int) -> None:
        if not 1 <= dim <= MAX_DIM:
            raise ValueError(f"embedding dim must be between 1 and {MAX_DIM}, got {dim}")
        self.dim = dim

    @property
    def model_id(self) -> str:
        return MODEL_ID

    def embed(self, texts: list[str]) -> list[list[float]]:
        return [self._vector(text) for text in texts]

    def _vector(self, text: str) -> list[float]:
        out: list[float] = []
        buf = hashlib.sha256(f"{MODEL_ID}\x00{text}".encode("utf-8")).digest()
        while len(out) < self.dim:
            for byte in buf:
                if len(out) >= self.dim:
                    break
                out.append((byte - 128 if byte < 128 else byte - 256) / 128.0)
            buf = hashlib.sha256(buf).digest()
        return self._normalize(out)

    @staticmethod
    def _normalize(vector: list[float]) -> list[float]:
        norm = math.sqrt(sum(x * x for x in vector))
        if norm == 0:
            return vector
        return [x / norm for x in vector]


class OpenAIEmbedder:
    def __init__(self, base_url: str, api_key: str, model: str, dim: int) -> None:
        if not base_url or not api_key or not model or not 1 <= dim <= 10_000:
            raise ValueError("OpenAI embedding base URL, API key, model, and dimension are required")
        self._base_url = base_url.rstrip("/")
        self._api_key = api_key
        self._model = model
        self._dim = dim

    @property
    def model_id(self) -> str:
        return self._model

    def embed(self, texts: list[str]) -> list[list[float]]:
        if not texts:
            raise ValueError("no texts provided")
        request = urllib.request.Request(
            self._base_url + "/embeddings",
            data=json.dumps({"model": self._model, "input": texts, "dimensions": self._dim}).encode("utf-8"),
            headers={"Authorization": "Bearer " + self._api_key, "Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                payload = json.load(response)
        except (urllib.error.URLError, OSError, json.JSONDecodeError) as exc:
            raise RuntimeError("embedding provider request failed") from exc
        rows = payload.get("data") if isinstance(payload, dict) else None
        if not isinstance(rows, list) or len(rows) != len(texts):
            raise RuntimeError("embedding provider returned an invalid vector count")
        vectors: list[list[float] | None] = [None] * len(texts)
        for position, row in enumerate(rows):
            if not isinstance(row, dict) or not isinstance(row.get("embedding"), list):
                raise RuntimeError("embedding provider returned an invalid vector")
            index = row.get("index")
            if not isinstance(index, int):
                index = position
            vector = row["embedding"]
            if index < 0 or index >= len(vectors) or vectors[index] is not None or len(vector) != self._dim or any(not isinstance(value, (int, float)) for value in vector):
                raise RuntimeError("embedding provider returned an invalid vector")
            if len(vector) != self._dim or any(not isinstance(value, (int, float)) for value in vector):
                raise RuntimeError("embedding provider returned an invalid vector dimension")
            vectors[index] = [float(value) for value in vector]
        if any(vector is None for vector in vectors):
            raise RuntimeError("embedding provider returned missing vectors")
        return [vector for vector in vectors if vector is not None]
