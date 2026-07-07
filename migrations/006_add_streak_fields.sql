-- migrate up
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS current_streak INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS longest_streak INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_users_last_active_at ON users(last_active_at);

-- migrate down
ALTER TABLE users
  DROP COLUMN IF EXISTS last_active_at,
  DROP COLUMN IF EXISTS longest_streak,
  DROP COLUMN IF EXISTS current_streak;
