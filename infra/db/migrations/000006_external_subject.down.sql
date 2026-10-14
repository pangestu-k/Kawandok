DROP INDEX IF EXISTS idx_users_external_subject;
ALTER TABLE users DROP COLUMN IF EXISTS external_subject;
