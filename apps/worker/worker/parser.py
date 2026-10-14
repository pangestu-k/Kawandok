from __future__ import annotations

import io
from dataclasses import dataclass

MIME_PDF = "application/pdf"
MIME_DOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
MIME_XLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"


class PermanentError(Exception):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


@dataclass(frozen=True)
class SourceUnit:
    content: str
    page_start: int | None
    page_end: int | None
    section_ref: str


def parse_document(mime_type: str, data: bytes) -> list[SourceUnit]:
    if mime_type == MIME_PDF:
        return _parse_pdf(data)
    if mime_type == MIME_DOCX:
        return _parse_docx(data)
    raise PermanentError("UNSUPPORTED_TYPE", f"no parser for mime type {mime_type!r}")


def _parse_pdf(data: bytes) -> list[SourceUnit]:
    try:
        import pypdf

        reader = pypdf.PdfReader(io.BytesIO(data))
        units: list[SourceUnit] = []
        for number, page in enumerate(reader.pages, start=1):
            text = _normalize(page.extract_text() or "")
            if text:
                units.append(SourceUnit(text, number, number, f"page:{number}"))
    except PermanentError:
        raise
    except Exception as exc:
        raise PermanentError("PDF_PARSE_FAILED", f"could not parse pdf: {exc}") from exc
    if not units:
        raise PermanentError("EMPTY_DOCUMENT", "pdf contains no extractable text")
    return units


def _parse_docx(data: bytes) -> list[SourceUnit]:
    try:
        import docx

        document = docx.Document(io.BytesIO(data))
        units: list[SourceUnit] = []
        for index, paragraph in enumerate(document.paragraphs, start=1):
            text = _normalize(paragraph.text)
            if text:
                units.append(SourceUnit(text, None, None, f"para:{index}"))
        for table_index, table in enumerate(document.tables, start=1):
            for row_index, row in enumerate(table.rows, start=1):
                cells = [cell.text.strip() for cell in row.cells]
                text = _normalize(" | ".join(cells))
                if text:
                    units.append(SourceUnit(text, None, None, f"table:{table_index}:row:{row_index}"))
    except PermanentError:
        raise
    except Exception as exc:
        raise PermanentError("DOCX_PARSE_FAILED", f"could not parse docx: {exc}") from exc
    if not units:
        raise PermanentError("EMPTY_DOCUMENT", "docx contains no extractable text")
    return units


def _normalize(text: str) -> str:
    return " ".join(text.split())
