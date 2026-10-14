from __future__ import annotations

import unittest

from worker.embedder import MockEmbedder


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


if __name__ == "__main__":
    unittest.main()
