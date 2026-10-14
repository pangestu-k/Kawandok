from __future__ import annotations

import datetime
import io
from dataclasses import dataclass

from worker.parser import PermanentError

MIME_XLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

REQUIRED_REPORT_FIELDS = ("period_start", "period_end", "currency", "department")
REQUIRED_TX_COLUMNS = ("transaction_date", "category", "amount", "currency")

MAX_ROWS = 10_000


@dataclass(frozen=True)
class ParsedReport:
    period_start: datetime.date
    period_end: datetime.date
    currency: str
    department: str
    rows: tuple["ParsedTransaction", ...]


@dataclass(frozen=True)
class ParsedTransaction:
    transaction_date: datetime.date
    category: str
    amount_minor_units: int
    currency: str
    source_sheet: str
    source_row: int
    external_transaction_ref: str


def parse_finance_xlsx(data: bytes) -> ParsedReport:
    try:
        import openpyxl

        workbook = openpyxl.load_workbook(io.BytesIO(data), read_only=True, data_only=True)
    except PermanentError:
        raise
    except Exception as exc:
        raise PermanentError("XLSX_PARSE_FAILED", f"could not open xlsx: {exc}") from exc

    if "Report" not in workbook.sheetnames or "Transactions" not in workbook.sheetnames:
        raise PermanentError(
            "XLSX_TEMPLATE_INVALID",
            "workbook must contain sheets named 'Report' and 'Transactions'",
        )

    report_fields = _parse_report_sheet(workbook["Report"])
    rows = _parse_transactions_sheet(workbook["Transactions"], report_fields["currency"])
    return ParsedReport(
        period_start=report_fields["period_start"],
        period_end=report_fields["period_end"],
        currency=report_fields["currency"],
        department=report_fields["department"],
        rows=tuple(rows),
    )


def _parse_report_sheet(sheet) -> dict:
    values: dict[str, object] = {}
    for row in sheet.iter_rows(max_row=50):
        key_cell = row[0].value if row else None
        if isinstance(key_cell, str):
            values[key_cell.strip().lower()] = row[1].value if len(row) > 1 else None

    missing = [f for f in REQUIRED_REPORT_FIELDS if f not in values or values[f] in (None, "")]
    if missing:
        raise PermanentError(
            "XLSX_TEMPLATE_INVALID",
            f"Report sheet is missing fields: {', '.join(missing)}",
        )

    period_start = _coerce_date(values["period_start"], "Report!period_start")
    period_end = _coerce_date(values["period_end"], "Report!period_end")
    if period_start > period_end:
        raise PermanentError("XLSX_VALIDATION_FAILED", "period_start is after period_end")

    currency = str(values["currency"]).strip().upper()
    if not (len(currency) == 3 and currency.isalpha()):
        raise PermanentError("XLSX_VALIDATION_FAILED", f"currency {currency!r} must be a 3-letter ISO code")

    department = str(values["department"]).strip()
    if not department:
        raise PermanentError("XLSX_VALIDATION_FAILED", "department must not be empty")

    return {
        "period_start": period_start,
        "period_end": period_end,
        "currency": currency,
        "department": department,
    }


def _parse_transactions_sheet(sheet, report_currency: str) -> list[ParsedTransaction]:
    rows_iter = sheet.iter_rows(values_only=True)
    header = next(rows_iter, None)
    if header is None:
        raise PermanentError("XLSX_TEMPLATE_INVALID", "Transactions sheet is empty")

    columns: dict[str, int] = {}
    for index, cell in enumerate(header):
        if isinstance(cell, str):
            name = cell.strip().lower()
            if name in REQUIRED_TX_COLUMNS or name == "external_transaction_ref":
                columns[name] = index
    missing = [c for c in REQUIRED_TX_COLUMNS if c not in columns]
    if missing:
        raise PermanentError(
            "XLSX_TEMPLATE_INVALID",
            f"Transactions sheet is missing columns: {', '.join(missing)}",
        )

    parsed: list[ParsedTransaction] = []
    seen_refs: dict[str, int] = {}
    for row_number, row in enumerate(rows_iter, start=2):
        if row_number > MAX_ROWS:
            raise PermanentError("XLSX_VALIDATION_FAILED", f"more than {MAX_ROWS} transaction rows")
        if row is None or all(cell is None or str(cell).strip() == "" for cell in row):
            continue

        def cell(name: str):
            index = columns[name]
            return row[index] if index < len(row) else None

        tx_date = _coerce_date(cell("transaction_date"), f"Transactions!row {row_number}")
        category = str(cell("category") or "").strip()
        if not category:
            raise PermanentError("XLSX_VALIDATION_FAILED", f"Transactions!row {row_number}: category is empty")

        raw_amount = cell("amount")
        if isinstance(raw_amount, bool) or not isinstance(raw_amount, (int, float)):
            raise PermanentError(
                "XLSX_VALIDATION_FAILED",
                f"Transactions!row {row_number}: amount must be a number",
            )
        amount_minor = _to_minor_units(float(raw_amount), f"Transactions!row {row_number}")

        currency = str(cell("currency") or "").strip().upper()
        if currency != report_currency:
            raise PermanentError(
                "XLSX_VALIDATION_FAILED",
                f"Transactions!row {row_number}: currency {currency!r} does not match report currency {report_currency!r}",
            )

        ref_cell = cell("external_transaction_ref") if "external_transaction_ref" in columns else None
        ref = str(ref_cell).strip() if ref_cell is not None else ""
        if ref:
            if ref in seen_refs:
                raise PermanentError(
                    "XLSX_VALIDATION_FAILED",
                    f"Transactions!row {row_number}: duplicate reference {ref!r} (also row {seen_refs[ref]})",
                )
            seen_refs[ref] = row_number

        parsed.append(
            ParsedTransaction(
                transaction_date=tx_date,
                category=category,
                amount_minor_units=amount_minor,
                currency=currency,
                source_sheet="Transactions",
                source_row=row_number,
                external_transaction_ref=ref,
            )
        )

    if not parsed:
        raise PermanentError("XLSX_VALIDATION_FAILED", "Transactions sheet has no data rows")
    return parsed


def _coerce_date(value, where: str) -> datetime.date:
    if isinstance(value, datetime.datetime):
        return value.date()
    if isinstance(value, datetime.date):
        return value
    text = str(value or "").strip()
    for fmt in ("%Y-%m-%d", "%d/%m/%Y", "%d-%m-%Y"):
        try:
            return datetime.datetime.strptime(text, fmt).date()
        except ValueError:
            continue
    raise PermanentError("XLSX_VALIDATION_FAILED", f"{where}: invalid date {text!r}")


def _to_minor_units(amount: float, where: str) -> int:
    if amount != amount or amount in (float("inf"), float("-inf")):
        raise PermanentError("XLSX_VALIDATION_FAILED", f"{where}: amount is not a finite number")
    minor = round(amount * 100)
    if abs(minor) > 2**62:
        raise PermanentError("XLSX_VALIDATION_FAILED", f"{where}: amount is out of range")
    return int(minor)
