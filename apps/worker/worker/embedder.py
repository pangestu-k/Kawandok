from __future__ import annotations

import hashlib
import math

MODEL_ID = "mock-embedding-v1"
MAX_DIM = 4096


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
