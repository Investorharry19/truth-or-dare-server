-- Migration 003: Add refresh token storage to users
ALTER TABLE users
ADD COLUMN refresh_token TEXT;

CREATE INDEX IF NOT EXISTS idx_users_refresh_token ON users(refresh_token);
