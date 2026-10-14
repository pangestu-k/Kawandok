from __future__ import annotations

import datetime
import unittest

try:
    import openpyxl

    HAVE_OPENPYXL = True
except ImportError:
    HAVE_OPENPYXL = False

from worker.finance import PermanentError, parse_finance_xlsx


def build_xlsx(report: dict | None = None, rows: list[tuple] | None = None,
               skip_transactions: bool = False) -> bytes:
    import io

    wb = openpyxl.Workbook()
    ws_report = wb.active
    ws_report.title = "Report"
    if report is not None:
        ws_report.append(["field", "value"])
        for key, value in report.items():
            ws_report.append([key, value])

    if not skip_transactions:
        ws_tx = wb.create_sheet("Transactions")
        ws_tx.append(["transaction_date", "category", "amount", "currency", "external_transaction_ref"])
        for row in rows or []:
            ws_tx.append(list(row))

    buffer = io.BytesIO()
    wb.save(buffer)
    return buffer.getvalue()


def default_report() -> dict:
    return {
        "period_start": datetime.date(2026, 1, 1),
        "period_end": datetime.date(2026, 2, 28),
        "currency": "IDR",
        "department": "Finance",
    }


def default_rows() -> list[tuple]:
    return [
        (datetime.date(2026, 1, 10), "revenue", 12_000_000, "IDR", "TRX-001"),
        (datetime.date(2026, 1, 15), "transport", 5_000_000, "IDR", "TRX-002"),
        (datetime.date(2026, 2, 20), "revenue", 15_000_000, "IDR", "TRX-003"),
    ]


@unittest.skipUnless(HAVE_OPENPYXL, "openpyxl not installed")
class ParseFinanceXlsxTests(unittest.TestCase):
    def test_valid_workbook_parses(self) -> None:
        parsed = parse_finance_xlsx(build_xlsx(default_report(), default_rows()))
        self.assertEqual(parsed.currency, "IDR")
        self.assertEqual(parsed.department, "Finance")
        self.assertEqual(parsed.period_start, datetime.date(2026, 1, 1))
        self.assertEqual(len(parsed.rows), 3)
        self.assertEqual(parsed.rows[0].amount_minor_units, 1_200_000_000)
        self.assertEqual(parsed.rows[1].amount_minor_units, 500_000_000)
        self.assertEqual(parsed.rows[0].source_row, 2)
        self.assertEqual(parsed.rows[0].source_sheet, "Transactions")
        self.assertEqual(parsed.rows[2].external_transaction_ref, "TRX-003")

    def test_negative_and_decimal_amounts(self) -> None:
        rows = [(datetime.date(2026, 1, 10), "correction", -1250.55, "IDR", "TRX-NEG")]
        parsed = parse_finance_xlsx(build_xlsx(default_report(), rows))
        self.assertEqual(parsed.rows[0].amount_minor_units, -125_055)

    def test_missing_sheet_fails(self) -> None:
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(build_xlsx(default_report(), default_rows(), skip_transactions=True))
        self.assertEqual(ctx.exception.code, "XLSX_TEMPLATE_INVALID")

    def test_missing_report_field_fails(self) -> None:
        report = default_report()
        del report["currency"]
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(build_xlsx(report, default_rows()))
        self.assertEqual(ctx.exception.code, "XLSX_TEMPLATE_INVALID")

    def test_missing_column_fails(self) -> None:
        import io
        data = build_xlsx(default_report(), default_rows())
        wb = openpyxl.load_workbook(io.BytesIO(data))
        wb["Transactions"].delete_cols(3)
        buffer = io.BytesIO()
        wb.save(buffer)
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(buffer.getvalue())
        self.assertEqual(ctx.exception.code, "XLSX_TEMPLATE_INVALID")

    def test_currency_mismatch_fails(self) -> None:
        rows = [(datetime.date(2026, 1, 10), "revenue", 100, "USD", "TRX-USD")]
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(build_xlsx(default_report(), rows))
        self.assertEqual(ctx.exception.code, "XLSX_VALIDATION_FAILED")

    def test_duplicate_reference_fails(self) -> None:
        rows = [
            (datetime.date(2026, 1, 10), "revenue", 100, "IDR", "TRX-DUP"),
            (datetime.date(2026, 1, 11), "revenue", 200, "IDR", "TRX-DUP"),
        ]
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(build_xlsx(default_report(), rows))
        self.assertEqual(ctx.exception.code, "XLSX_VALIDATION_FAILED")
        self.assertIn("TRX-DUP", ctx.exception.__str__())

    def test_non_numeric_amount_fails(self) -> None:
        rows = [(datetime.date(2026, 1, 10), "revenue", "twelve million", "IDR", "TRX-BAD")]
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(build_xlsx(default_report(), rows))
        self.assertEqual(ctx.exception.code, "XLSX_VALIDATION_FAILED")

    def test_unparseable_date_fails(self) -> None:
        rows = [("not-a-date", "revenue", 100, "IDR", "TRX-BAD")]
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(build_xlsx(default_report(), rows))
        self.assertEqual(ctx.exception.code, "XLSX_VALIDATION_FAILED")

    def test_string_dates_are_accepted(self) -> None:
        rows = [("2026-01-10", "revenue", 100, "IDR", "TRX-STR")]
        parsed = parse_finance_xlsx(build_xlsx(default_report(), rows))
        self.assertEqual(parsed.rows[0].transaction_date, datetime.date(2026, 1, 10))

    def test_period_inversion_fails(self) -> None:
        report = {
            "period_start": datetime.date(2026, 3, 1),
            "period_end": datetime.date(2026, 1, 1),
            "currency": "IDR",
            "department": "Finance",
        }
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(build_xlsx(report, default_rows()))
        self.assertEqual(ctx.exception.code, "XLSX_VALIDATION_FAILED")

    def test_empty_transactions_fail(self) -> None:
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(build_xlsx(default_report(), []))
        self.assertEqual(ctx.exception.code, "XLSX_VALIDATION_FAILED")

    def test_corrupt_bytes_fail(self) -> None:
        with self.assertRaises(PermanentError) as ctx:
            parse_finance_xlsx(b"PK\x03\x04 not a real xlsx")
        self.assertEqual(ctx.exception.code, "XLSX_PARSE_FAILED")


if __name__ == "__main__":
    unittest.main()
