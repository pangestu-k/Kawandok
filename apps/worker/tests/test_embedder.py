from __future__ import annotations

import io
import unittest
from unittest.mock import patch

from worker.embedder import MockEmbedder, OpenAIEmbedder


class MockEmbedderTests(unittest.TestCase):
    def test_deterministic(self) -> None:
        embedder = MockEmbedder(64)
        first = embedder.embed(["leave policy"])
        second = embedder.embed(["leave policy"])
        self.assertEqual(first, second)

    def test_different_texts_differ(self) -> None:
        vectors = MockEmbedder(64).embed(["leave policy", "invoice total"])
        self.assertNotEqual(vectors[0], vectors[1])

    def test_unit_norm(self) -> None:
        for dim in (1, 8, 1536):
            vectors = MockEmbedder(dim).embed(["norm check"])
            norm = sum(x * x for x in vectors[0]) ** 0.5
            self.assertAlmostEqual(norm, 1.0, places=5)

    def test_dim_validation(self) -> None:
        with self.assertRaises(ValueError):
            MockEmbedder(0)
        with self.assertRaises(ValueError):
            MockEmbedder(4097)

    def test_model_id(self) -> None:
        self.assertEqual(MockEmbedder(8).model_id, "mock-embedding-v1")

    def test_openai_embedder_preserves_response_order(self) -> None:
        class Response:
            def __enter__(self):
                return io.BytesIO(b'{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}]}')

            def __exit__(self, *args):
                return False

        with patch("urllib.request.urlopen", return_value=Response()):
            vectors = OpenAIEmbedder("https://example.test/v1", "key", "model", 2).embed(["one", "two"])
        self.assertEqual(vectors, [[1.0, 2.0], [3.0, 4.0]])


if __name__ == "__main__":
    unittest.main()
