from __future__ import annotations

import unittest

try:
    import docx  # noqa: F401
    import pypdf  # noqa: F401

    HAVE_PARSER_DEPS = True
except ImportError:
    HAVE_PARSER_DEPS = False

from worker.parser import MIME_DOCX, MIME_PDF, PermanentError, SourceUnit, parse_document


def make_pdf(lines: list[str]) -> bytes:
    page_count = len(lines)
    kids = []
    bodies = []
    for index in range(1, page_count + 1):
        page_obj = 3 + 2 * index - 1
        content_obj = 3 + 2 * index
        kids.append(f"{page_obj} 0 R")
        escaped = lines[index - 1].replace("\\", r"\\").replace("(", r"\(").replace(")", r"\)")
        stream = f"BT /F1 12 Tf 72 720 Td ({escaped}) Tj ET"
        bodies.append(
            f"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents {content_obj} 0 R "
            f"/Resources << /Font << /F1 3 0 R >> >> >>"
        )
        bodies.append(f"<< /Length {len(stream)} >>\nstream\n{stream}\nendstream")

    objects = [
        "<< /Type /Catalog /Pages 2 0 R >>",
        f"<< /Type /Pages /Kids [{' '.join(kids)}] /Count {page_count} >>",
        "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
        *bodies,
    ]
    out = bytearray(b"%PDF-1.4\n")
    offsets = []
    for i, obj in enumerate(objects, start=1):
        offsets.append(len(out))
        out += f"{i} 0 obj\n{obj}\nendobj\n".encode("latin-1")
    xref_pos = len(out)
    out += f"xref\n0 {len(objects) + 1}\n".encode()
    out += b"0000000000 65535 f \n"
    for off in offsets:
        out += f"{off:010d} 00000 n \n".encode()
    out += (
        f"trailer\n<< /Size {len(objects) + 1} /Root 1 0 R >>\n"
        f"startxref\n{xref_pos}\n%%EOF\n"
    ).encode()
    return bytes(out)


def make_docx(paragraphs: list[str], table_rows: list[list[str]] | None = None) -> bytes:
    import io

    import docx

    document = docx.Document()
    for text in paragraphs:
        document.add_paragraph(text)
    if table_rows:
        table = document.add_table(rows=len(table_rows), cols=len(table_rows[0]))
        for r, row in enumerate(table_rows):
            for c, cell in enumerate(row):
                table.rows[r].cells[c].text = cell
    buffer = io.BytesIO()
    document.save(buffer)
    return buffer.getvalue()


@unittest.skipUnless(HAVE_PARSER_DEPS, "pypdf/python-docx not installed")
class ParseDocumentTests(unittest.TestCase):
    def test_pdf_pages_have_provenance(self) -> None:
        data = make_pdf(["Total revenue is 32 million IDR.", "Second page content."])
        units = parse_document(MIME_PDF, data)
        self.assertEqual(len(units), 2)
        self.assertEqual(units[0].page_start, 1)
        self.assertEqual(units[0].page_end, 1)
        self.assertEqual(units[0].section_ref, "page:1")
        self.assertEqual(units[1].page_start, 2)
        self.assertIn("Second page", units[1].content)

    def test_docx_paragraphs_and_tables(self) -> None:
        data = make_docx(
            ["First paragraph about leave policy.", "Second paragraph."],
            [["Category", "Amount"], ["Transport", "5000000"]],
        )
        units = parse_document(MIME_DOCX, data)
        refs = [u.section_ref for u in units]
        self.assertIn("para:1", refs)
        self.assertIn("para:2", refs)
        self.assertIn("table:1:row:1", refs)
        table_unit = next(u for u in units if u.section_ref == "table:1:row:2")
        self.assertIn("Transport", table_unit.content)
        self.assertIn("5000000", table_unit.content)

    def test_unsupported_mime_rejected(self) -> None:
        with self.assertRaises(PermanentError) as ctx:
            parse_document("application/msword", b"whatever")
        self.assertEqual(ctx.exception.code, "UNSUPPORTED_TYPE")

    def test_corrupt_pdf_fails_permanently(self) -> None:
        with self.assertRaises(PermanentError) as ctx:
            parse_document(MIME_PDF, b"%PDF-1.4 this is not really a pdf")
        self.assertEqual(ctx.exception.code, "PDF_PARSE_FAILED")

    def test_empty_docx_fails_permanently(self) -> None:
        data = make_docx([])
        with self.assertRaises(PermanentError) as ctx:
            parse_document(MIME_DOCX, data)
        self.assertEqual(ctx.exception.code, "EMPTY_DOCUMENT")

    def test_whitespace_is_normalized(self) -> None:
        data = make_docx(["multi   spaces\t\tand\nnewlines here"])
        units = parse_document(MIME_DOCX, data)
        self.assertEqual(units[0].content, "multi spaces and newlines here")


if __name__ == "__main__":
    unittest.main()
