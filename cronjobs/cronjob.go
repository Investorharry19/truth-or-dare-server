package cronjobs

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ==================== CRON JOB: Reset Free Points Every 24h ====================

// StartPointsResetCron starts the cron job to reset free points every 24 hours
func StartPointsResetCron(db *pgxpool.Pool) {
	ticker := time.NewTicker(24 * time.Hour)

	// Run immediately on startup (optional)
	go resetAllFreePoints(db)

	go func() {
		for range ticker.C {
			resetAllFreePoints(db)
		}
	}()

	log.Println("Points reset cron job started (runs every 24 hours)")
}

// resetAllFreePoints resets free points to 30 for all users
func resetAllFreePoints(db *pgxpool.Pool) {
	log.Println("Running free points reset...")

	query := `
		UPDATE users 
		SET free_points = 30, 
		    last_reset_at = NOW()
		WHERE last_reset_at < NOW() - INTERVAL '24 hours'
		   OR last_reset_at IS NULL
	`
	ctx := context.TODO()
	result, err := db.Exec(ctx, query)
	if err != nil {
		log.Printf("Error resetting free points: %v\n", err)
		return
	}

	rowsAffected := result.RowsAffected()
	log.Printf("Reset free points for %d users\n", rowsAffected)
}

// ==================== INDIVIDUAL USER RESET (if needed) ====================

// resetUserFreePoints resets a specific user's free points if 24h passed
// func resetUserFreePoints(db *sql.DB, userID string) error {
// 	query := `
// 		UPDATE users
// 		SET free_points = 30,
// 		    last_reset_at = NOW()
// 		WHERE id = $1
// 		  AND (last_reset_at < NOW() - INTERVAL '24 hours' OR last_reset_at IS NULL)
// 		RETURNING free_points
// 	`

// 	var newFreePoints int
// 	err := db.QueryRow(query, userID).Scan(&newFreePoints)

// 	if err == sql.ErrNoRows {
// 		// No reset needed (less than 24h since last reset)
// 		return nil
// 	}

// 	if err != nil {
// 		return fmt.Errorf("failed to reset free points: %w", err)
// 	}

// 	log.Printf(" Reset free points for user %s to %d\n", userID, newFreePoints)
// 	return nil
// }

// ==================== GET USER TOTAL POINTS ====================

// getUserPoints gets total points (free + paid) for a user
func getUserPoints(db *sql.DB, userID string) (free int, paid int, total int, err error) {
	query := `SELECT free_points, paid_points FROM users WHERE id = $1`

	err = db.QueryRow(query, userID).Scan(&free, &paid)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get user points: %w", err)
	}

	total = free + paid
	return free, paid, total, nil
}

// ==================== DEDUCT POINTS (Priority: Paid first, then Free) ====================

// deductPoints deducts points from user (paid first, then free)
func deductPoints(db *sql.DB, userID string, amount int) error {
	// Get current points
	free, paid, total, err := getUserPoints(db, userID)
	if err != nil {
		return err
	}

	if total < amount {
		return fmt.Errorf("insufficient points: has %d, needs %d", total, amount)
	}

	// Deduct from paid first
	newPaid := paid
	newFree := free
	remaining := amount

	if paid >= remaining {
		// Enough paid points
		newPaid = paid - remaining
		remaining = 0
	} else {
		// Use all paid points, then deduct from free
		newPaid = 0
		remaining -= paid
		newFree = free - remaining
	}

	// Update database
	query := `
		UPDATE users 
		SET paid_points = $1, free_points = $2
		WHERE id = $3
	`

	_, err = db.Exec(query, newPaid, newFree, userID)
	if err != nil {
		return fmt.Errorf("failed to deduct points: %w", err)
	}

	log.Printf("💰 Deducted %d points from user %s (Paid: %d->%d, Free: %d->%d)\n",
		amount, userID, paid, newPaid, free, newFree)

	return nil
}

// ==================== ADD POINTS ====================

// addPaidPoints adds paid points to user

// addFreePoints adds free points to user (for transfers)

// ==================== MAIN FUNCTION (Example) ====================

// func main() {
// 	// Database connection
// 	connStr := "host=localhost port=5432 user=youruser password=yourpassword dbname=truthordare sslmode=disable"
// 	db, err := sql.Open("postgres", connStr)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	defer db.Close()

// 	// Test connection
// 	if err = db.Ping(); err != nil {
// 		log.Fatal(err)
// 	}

// 	log.Println("Database connected")

// }
