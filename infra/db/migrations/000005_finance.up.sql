CREATE TABLE financial_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_version_id UUID NOT NULL UNIQUE REFERENCES document_versions(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES departments(id),
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    currency CHAR(3) NOT NULL,
    validation_status TEXT NOT NULL DEFAULT 'validated' CHECK (validation_status IN ('validated', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (period_start <= period_end)
);

CREATE TABLE financial_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id UUID NOT NULL REFERENCES financial_reports(id) ON DELETE CASCADE,
    transaction_date DATE NOT NULL,
    category TEXT NOT NULL,
    amount_minor_units BIGINT NOT NULL,
    currency CHAR(3) NOT NULL,
    source_sheet TEXT NOT NULL,
    source_row INTEGER NOT NULL,
    external_transaction_ref TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_financial_transactions_report ON financial_transactions(report_id, transaction_date);
CREATE INDEX idx_financial_reports_dept_period ON financial_reports(department_id, period_start, period_end);
CREATE UNIQUE INDEX idx_financial_tx_ref
    ON financial_transactions(report_id, external_transaction_ref)
    WHERE external_transaction_ref <> '';
