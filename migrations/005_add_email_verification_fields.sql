-- migrate up
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS verification_token TEXT,
  ADD COLUMN IF NOT EXISTS verification_expires_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_users_verification_token ON users(verification_token);

-- migrate down
ALTER TABLE users
  DROP COLUMN IF EXISTS verification_token,
  DROP COLUMN IF EXISTS verification_expires_at,
  DROP COLUMN IF EXISTS email_verified;