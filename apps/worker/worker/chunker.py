from __future__ import annotations

import math
import re
from dataclasses import dataclass

from worker.parser import SourceUnit

_SENTENCE_SPLIT = re.compile(r"(?<=[.!?])\s+")


@dataclass(frozen=True)
class Chunk:
    content: str
    page_start: int | None
    page_end: int | None
    section_ref: str
    token_count: int


def chunk_units(
    units: list[SourceUnit],
    target_chars: int = 1200,
    overlap_chars: int | None = None,
) -> list[Chunk]:
    if target_chars < 32:
        raise ValueError("target_chars must be at least 32")
    if overlap_chars is None:
        overlap_chars = max(0, target_chars // 10)
    if overlap_chars < 0 or overlap_chars >= target_chars:
        raise ValueError("overlap_chars must be smaller than target_chars")

    atomic: list[SourceUnit] = []
    for unit in units:
        if not unit.content:
            continue
        if len(unit.content) <= target_chars:
            atomic.append(unit)
            continue
        atomic.extend(_split_unit(unit, target_chars, overlap_chars))

    chunks: list[Chunk] = []
    current: list[SourceUnit] = []
    for unit in atomic:
        projected = len(unit.content) + (1 if current else 0)
        if current and _size(current) + projected > target_chars:
            chunks.append(_make_chunk(current))
            current = []
        current.append(unit)
    if current:
        chunks.append(_make_chunk(current))
    return chunks


def _split_unit(unit: SourceUnit, target_chars: int, overlap_chars: int) -> list[SourceUnit]:
    sentences = [s for s in _SENTENCE_SPLIT.split(unit.content) if s]
    if not sentences:
        sentences = [unit.content]

    pieces: list[str] = []
    current: list[str] = []
    size = 0
    for sentence in sentences:
        if current and size + len(sentence) + 1 > target_chars:
            pieces.append(" ".join(current))
            tail = _overlap_tail(pieces[-1], overlap_chars)
            current = [tail, sentence] if tail else [sentence]
            size = len(" ".join(current))
            continue
        current.append(sentence)
        size += len(sentence) + (1 if size else 0)
    if current:
        pieces.append(" ".join(current))

    return [
        SourceUnit(piece, unit.page_start, unit.page_end, f"{unit.section_ref}:{index}")
        for index, piece in enumerate(pieces, start=1)
    ]


def _overlap_tail(text: str, overlap_chars: int) -> str:
    if overlap_chars <= 0 or len(text) <= overlap_chars:
        return ""
    cut = text[-overlap_chars:]
    space = cut.find(" ")
    if space != -1:
        cut = cut[space + 1 :]
    return cut.strip()


def _size(units: list[SourceUnit]) -> int:
    return sum(len(u.content) for u in units) + max(0, len(units) - 1)


def _make_chunk(units: list[SourceUnit]) -> Chunk:
    content = "\n".join(u.content for u in units)
    pages = [u.page_start for u in units if u.page_start is not None]
    pages_end = [u.page_end for u in units if u.page_end is not None]
    return Chunk(
        content=content,
        page_start=min(pages) if pages else None,
        page_end=max(pages_end) if pages_end else None,
        section_ref=units[0].section_ref,
        token_count=max(1, math.ceil(len(content) / 4)),
    )
