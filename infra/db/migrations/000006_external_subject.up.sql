ALTER TABLE users ADD COLUMN external_subject TEXT;
CREATE UNIQUE INDEX idx_users_external_subject ON users(external_subject) WHERE external_subject IS NOT NULL;
