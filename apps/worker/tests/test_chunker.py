from __future__ import annotations

import unittest

from worker.chunker import Chunk, chunk_units
from worker.parser import SourceUnit


def unit(content: str, page: int | None = None, ref: str | None = None) -> SourceUnit:
    return SourceUnit(
        content=content,
        page_start=page,
        page_end=page,
        section_ref=ref or (f"page:{page}" if page else "para:1"),
    )


class ChunkerTests(unittest.TestCase):
    def test_packs_units_up_to_target(self) -> None:
        units = [unit("a" * 60, page=1), unit("b" * 60, page=2), unit("c" * 60, page=3)]
        chunks = chunk_units(units, target_chars=150, overlap_chars=10)
        self.assertEqual(len(chunks), 2)
        self.assertEqual(chunks[0].page_start, 1)
        self.assertEqual(chunks[0].page_end, 2)
        self.assertEqual(chunks[0].section_ref, "page:1")

    def test_page_range_spans_units(self) -> None:
        units = [unit("short one", page=2), unit("another short", page=5)]
        chunks = chunk_units(units, target_chars=200, overlap_chars=10)
        self.assertEqual(len(chunks), 1)
        self.assertEqual(chunks[0].page_start, 2)
        self.assertEqual(chunks[0].page_end, 5)

    def test_long_unit_is_split_with_overlap(self) -> None:
        long_text = "First sentence is here. Second sentence follows. Third sentence ends it. Fourth arrives."
        chunks = chunk_units([unit(long_text, page=7)], target_chars=60, overlap_chars=12)
        self.assertGreater(len(chunks), 1)
        joined = " ".join(c.content for c in chunks)
        for sentence in ("First sentence", "Second sentence", "Third sentence", "Fourth arrives"):
            self.assertIn(sentence, joined)
        self.assertEqual(chunks[0].page_start, 7)

    def test_token_count_is_positive_and_deterministic(self) -> None:
        chunks = chunk_units([unit("hello world this is a test", page=1)], target_chars=100)
        self.assertEqual(len(chunks), 1)
        self.assertGreater(chunks[0].token_count, 0)
        again = chunk_units([unit("hello world this is a test", page=1)], target_chars=100)
        self.assertEqual(chunks[0].token_count, again[0].token_count)

    def test_empty_units_are_skipped(self) -> None:
        self.assertEqual(chunk_units([unit("", page=1)], target_chars=100), [])

    def test_invalid_parameters_rejected(self) -> None:
        with self.assertRaises(ValueError):
            chunk_units([unit("x")], target_chars=10)
        with self.assertRaises(ValueError):
            chunk_units([unit("x")], target_chars=100, overlap_chars=100)
    def test_docx_units_have_no_pages(self) -> None:
        chunks = chunk_units([unit("text only", ref="para:3")], target_chars=100)
        self.assertIsNone(chunks[0].page_start)
        self.assertIsNone(chunks[0].page_end)
        self.assertEqual(chunks[0].section_ref, "para:3")

    def test_all_content_preserved(self) -> None:
        texts = [f"paragraph number {i} with content" for i in range(20)]
        chunks = chunk_units([unit(t, page=1) for t in texts], target_chars=80)
        joined = "\n".join(c.content for c in chunks)
        for text in texts:
            self.assertIn(text, joined)


if __name__ == "__main__":
    unittest.main()
