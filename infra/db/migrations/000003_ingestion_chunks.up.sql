CREATE TABLE document_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_version_id UUID NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL,
    content TEXT NOT NULL,
    page_start INTEGER,
    page_end INTEGER,
    section_ref TEXT,
    sheet_name TEXT,
    cell_range TEXT,
    token_count INTEGER NOT NULL DEFAULT 0,
    embedding vector({{EMBEDDING_DIM}}) NOT NULL,
    tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce(content, ''))) STORED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_version_id, ordinal)
);

CREATE INDEX idx_chunks_version ON document_chunks(document_version_id);
CREATE INDEX idx_chunks_tsv ON document_chunks USING GIN(tsv);

CREATE TABLE ingestion_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_version_id UUID NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
    job_type TEXT NOT NULL DEFAULT 'ingest' CHECK (job_type IN ('ingest', 'reindex')),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'ready', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    last_error_code TEXT,
    last_error_message TEXT,
    run_after TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_ingestion_jobs_active
    ON ingestion_jobs(document_version_id)
    WHERE status IN ('queued', 'processing');

CREATE TABLE ingestion_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES ingestion_jobs(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dispatched')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched_at TIMESTAMPTZ
);

CREATE INDEX idx_outbox_pending ON ingestion_outbox(created_at) WHERE status = 'pending';
CREATE INDEX idx_jobs_due ON ingestion_jobs(run_after) WHERE status = 'queued';
