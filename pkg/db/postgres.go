package db

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

const (
	maxAttempts    = 10               // total tries before giving up
	attemptTimeout = 10 * time.Second // time allowed for each try
	initialBackoff = 2 * time.Second  // wait before the 2nd try
	maxBackoff     = 30 * time.Second // backoff never grows past this
)

func Connect() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	// A malformed DSN will never succeed, so fail immediately instead of retrying.
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("Invalid DATABASE_URL: %v", err)
	}

	backoff := initialBackoff
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pool, err := tryConnect(cfg)
		if err == nil {
			Pool = pool
			log.Printf("Connected to PostgreSQL (attempt %d/%d)", attempt, maxAttempts)
			return
		}

		lastErr = err
		log.Printf("DB connection attempt %d/%d failed: %v", attempt, maxAttempts, err)

		if attempt < maxAttempts {
			log.Printf("Retrying in %s...", backoff)
			time.Sleep(backoff)

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	log.Fatalf("Unable to connect to DB after %d attempts: %v", maxAttempts, lastErr)
}

func tryConnect(cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), attemptTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close() // don't leak the failed pool
		return nil, err
	}

	return pool, nil
}

// Healthy reports whether the database currently answers a ping.
func Healthy() bool {
	if Pool == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return Pool.Ping(ctx) == nil
}
