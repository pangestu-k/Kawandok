DROP INDEX IF EXISTS idx_jobs_due;
DROP INDEX IF EXISTS idx_outbox_pending;
DROP TABLE IF EXISTS ingestion_outbox;
DROP TABLE IF EXISTS ingestion_jobs;
DROP INDEX IF EXISTS idx_chunks_tsv;
DROP INDEX IF EXISTS idx_chunks_version;
DROP TABLE IF EXISTS document_chunks;
