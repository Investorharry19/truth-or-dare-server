-- migrate up
ALTER TABLE users
  ADD COLUMN full_name TEXT;

-- migrate down
ALTER TABLE users
  DROP COLUMN full_name;