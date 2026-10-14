ALTER TABLE documents DROP CONSTRAINT documents_current_version_fk;

DROP INDEX IF EXISTS idx_users_department;
DROP INDEX IF EXISTS idx_audit_events_time;
DROP INDEX IF EXISTS idx_document_grants_lookup;
DROP INDEX IF EXISTS idx_document_versions_document;
DROP INDEX IF EXISTS idx_documents_deleted_at;

DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS document_grants;
DROP TABLE IF EXISTS document_versions;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS departments;
