ALTER TABLE users
    ADD COLUMN IF NOT EXISTS paid_points INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS free_points INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_reset_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS password_reset_token TEXT,
    ADD COLUMN IF NOT EXISTS password_reset_expires_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_users_password_reset_token ON users(password_reset_token);
