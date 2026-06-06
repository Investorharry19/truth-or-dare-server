package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ==================== CONFIGURATION ====================

const (
	PAYSTACK_API_URL    = "https://api.paystack.co"
	POINTS_PRICE        = 5 // 1 point = ₦5
	MIN_POINTS_PURCHASE = 120
)

var PAYSTACK_SECRET_KEY string
var PAYSTACK_PUBLIC_KEY string

// ==================== MODELS ====================

type InitializePaymentRequest struct {
	Points int `json:"points" binding:"required,min=120"`
}

type InitializePaymentResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		AuthorizationURL string `json:"authorization_url"`
		AccessCode       string `json:"access_code"`
		Reference        string `json:"reference"`
	} `json:"data"`
}

type PaystackWebhookPayload struct {
	Event string `json:"event"`
	Data  struct {
		Reference string `json:"reference"`
		Amount    int    `json:"amount"` // In kobo (₦1 = 100 kobo)
		Status    string `json:"status"`
		PaidAt    string `json:"paid_at"`
		Customer  struct {
			Email string `json:"email"`
		} `json:"customer"`
		Metadata struct {
			UserID string `json:"user_id"`
			Points string `json:"points"`
		} `json:"metadata"`
	} `json:"data"`
}

type Transaction struct {
	ID            string
	UserID        string
	Type          string // "purchase", "transfer", "deduction"
	Amount        int
	Points        int
	Reference     string
	Status        string // "pending", "success", "failed"
	PaymentMethod string
	CreatedAt     string
}

// ==================== SETUP ROUTES ====================

// ==================== INITIALIZE PAYMENT ====================

func InitializePayment(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found in context"})
			return
		}

		var req InitializePaymentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request. Minimum purchase is 120 points",
			})
			return
		}

		// Validate minimum points
		if req.Points < MIN_POINTS_PURCHASE {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Minimum purchase is %d points", MIN_POINTS_PURCHASE),
			})
			return
		}

		// Calculate amount in kobo (₦1 = 100 kobo)
		amountInNaira := req.Points * POINTS_PRICE
		amountInKobo := amountInNaira * 100

		// Get user email
		var userEmail string
		err := db.QueryRow(c.Request.Context(), "SELECT email FROM users WHERE id = $1", userID).Scan(&userEmail)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get user information",
			})
			return
		}

		// Generate unique reference
		reference := fmt.Sprintf("TRD_%s_%d", userID.(string)[:8], time.Now().Unix())
		fmt.Println(123)

		// Prepare Paystack request
		paystackReq := map[string]interface{}{
			"email":     userEmail,
			"amount":    amountInKobo,
			"reference": reference,
			"metadata": map[string]interface{}{
				"user_id": userID,
				"points":  req.Points,
			},
			"callback_url": "http://localhost:8080/payment/callback", // Optional
		}

		jsonData, _ := json.Marshal(paystackReq)

		httpReq, err := http.NewRequestWithContext(
			c.Request.Context(),
			http.MethodPost,
			PAYSTACK_API_URL+"/transaction/initialize",
			bytes.NewReader(jsonData),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to create payment request",
			})
			return
		}

		httpReq.Header.Set("Authorization", "Bearer "+PAYSTACK_SECRET_KEY)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Idempotency-Key", reference)

		client := &http.Client{}
		resp, err := client.Do(httpReq)
		if err != nil {
			fmt.Println(err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to initialize payment",
			})
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		fmt.Println(156)
		var paystackResp InitializePaymentResponse
		if err := json.Unmarshal(body, &paystackResp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to process payment response",
			})
			return
		}

		if !paystackResp.Status {
			fmt.Println(176, PAYSTACK_SECRET_KEY)
			fmt.Println(paystackResp)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": paystackResp.Message,
			})
			return
		}

		// Save transaction to database
		_, err = db.Exec(c.Request.Context(), `
			INSERT INTO transactions (id, user_id, type, amount, points, reference, status, payment_method, created_at)
			VALUES (gen_random_uuid(), $1, 'purchase', $2, $3, $4, 'pending', 'paystack', NOW())
		`, userID, amountInNaira, req.Points, reference)

		if err != nil {
			fmt.Printf("Warning: Failed to save transaction: %v\n", err)
		}

		c.JSON(http.StatusOK, gin.H{
			"status":            true,
			"authorization_url": paystackResp.Data.AuthorizationURL,
			"access_code":       paystackResp.Data.AccessCode,
			"reference":         reference,
			"amount":            amountInNaira,
			"points":            req.Points,
		})
	}
}

// ==================== VERIFY PAYMENT ====================

func VerifyPayment(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		reference := c.Param("reference")
		userID := c.GetString("user_id")
		ctx := c.Request.Context()
		// Call Paystack verification endpoint
		paystackURL := fmt.Sprintf("%s/transaction/verify/%s", PAYSTACK_API_URL, reference)
		httpReq, _ := http.NewRequest("GET", paystackURL, nil)
		httpReq.Header.Set("Authorization", "Bearer "+PAYSTACK_SECRET_KEY)

		client := &http.Client{}
		resp, err := client.Do(httpReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to verify payment",
			})
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		var verifyResp struct {
			Status  bool   `json:"status"`
			Message string `json:"message"`
			Data    struct {
				Status   string `json:"status"`
				Amount   int    `json:"amount"`
				Metadata struct {
					UserID string `json:"user_id"`
					Points int    `json:"points"`
				} `json:"metadata"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &verifyResp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to process verification response",
			})
			return
		}

		if !verifyResp.Status || verifyResp.Data.Status != "success" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Payment verification failed",
			})
			return
		}

		// Verify user owns this transaction
		if verifyResp.Data.Metadata.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Unauthorized",
			})
			return
		}

		// Add paid points to user
		points := verifyResp.Data.Metadata.Points
		err = addPaidPoints(ctx, db, userID, points)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to add points",
			})
			return
		}

		// Update transaction status
		db.Exec(ctx, `
			UPDATE transactions 
			SET status = 'success', updated_at = NOW()
			WHERE reference = $1
		`, reference)

		// Get updated balance
		free, paid, total, _ := getUserPoints(ctx, db, userID)

		c.JSON(http.StatusOK, gin.H{
			"status":       true,
			"message":      "Payment verified successfully",
			"points_added": points,
			"balance": gin.H{
				"free_points":  free,
				"paid_points":  paid,
				"total_points": total,
			},
		})
	}
}

// ==================== PAYSTACK WEBHOOK ====================

func PaystackWebhook(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Verify webhook signature
		signature := c.GetHeader("x-paystack-signature")
		ctx := c.Request.Context()

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		// Verify signature
		mac := hmac.New(sha512.New, []byte(PAYSTACK_SECRET_KEY))
		mac.Write(body)
		expectedSignature := hex.EncodeToString(mac.Sum(nil))

		if signature != expectedSignature {
			fmt.Println("Invalid webhook signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
			return
		}

		// Parse webhook payload
		var payload PaystackWebhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			fmt.Println(err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
			return
		}

		// Handle different event types
		switch payload.Event {
		case "charge.success":
			handleSuccessfulCharge(ctx, db, payload)
		default:
			fmt.Printf("Unhandled webhook event: %s\n", payload.Event)
		}

		c.JSON(http.StatusOK, gin.H{"status": "received"})
	}
}

// ==================== CallbacHk Handler ====================

func PaymentCallbackHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		html := `
			<!DOCTYPE html>
			<html lang="en">
				<head>
					<meta charset="UTF-8">
					<meta name="viewport" content="width=device-width, initial-scale=1.0">
					<title>Payment Callback</title>
					<style>
						* {
							margin: 0;
							padding: 0;
							box-sizing: border-box;
						}
						body {
							font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
							background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
							min-height: 100vh;
							display: flex;
							justify-content: center;
							align-items: center;
							padding: 20px;
						}
						.container {
							background: white;
							border-radius: 12px;
							box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
							padding: 40px;
							text-align: center;
							max-width: 500px;
						}
						.checkmark {
							width: 80px;
							height: 80px;
							margin: 0 auto 20px;
							background: #4CAF50;
							border-radius: 50%;
							display: flex;
							justify-content: center;
							align-items: center;
							font-size: 48px;
							color: white;
						}
						h1 {
							color: #333;
							margin-bottom: 10px;
							font-size: 28px;
						}
						p {
							color: #666;
							margin-bottom: 20px;
							font-size: 16px;
							line-height: 1.6;
						}
						.status {
							background: #f0f7ff;
							border-left: 4px solid #667eea;
							padding: 15px;
							text-align: left;
							margin: 20px 0;
							border-radius: 4px;
							font-size: 14px;
							color: #555;
						}
						.button {
							display: inline-block;
							background: #667eea;
							color: white;
							padding: 12px 30px;
							border-radius: 6px;
							text-decoration: none;
							font-weight: 600;
							margin-top: 20px;
							transition: background 0.3s;
							border: none;
							cursor: pointer;
							font-size: 16px;
						}
						.button:hover {
							background: #764ba2;
						}
					</style>
				</head>
					<body>
						<div class="container">
							<div class="checkmark">✓</div>
							<h1>Payment Callback Received</h1>
							<p>Your payment notification has been successfully received and is being processed.</p>
							<div class="status">
								<strong>Status:</strong> Callback received<br>
								<strong>Time:</strong> ` + time.Now().Format("2006-01-02 15:04:05") + `
							</div>
							<p style="font-size: 14px; color: #999; margin-top: 20px;">
								You can close this window or return to the application.
							</p>
							<button class="button" onclick="window.close()">Close</button>
						</div>
					</body>
			</html>
		`
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, html)
	}
}

func handleSuccessfulCharge(ctx context.Context, db *pgxpool.Pool, payload PaystackWebhookPayload) {
	userID := payload.Data.Metadata.UserID
	points := payload.Data.Metadata.Points
	reference := payload.Data.Reference

	fmt.Printf("Processing successful charge: User=%s, Points=%d, Ref=%s\n",
		userID, points, reference)

	// Check if already processed
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM transactions WHERE reference = $1 AND status = 'success')
	`, reference).Scan(&exists)

	if exists {
		fmt.Println("Transaction already processed")
		return
	}

	// Add paid points
	pointInt, _ := strconv.Atoi(points)
	if err := addPaidPoints(ctx, db, userID, pointInt); err != nil {
		fmt.Printf("Failed to add points: %v\n", err)
		return
	}

	// Update transaction status
	_, err = db.Exec(ctx, `
		UPDATE transactions 
		SET status = 'success', updated_at = NOW()
		WHERE reference = $1
	`, reference)

	if err != nil {
		fmt.Printf("Failed to update transaction: %v\n", err)
		return
	}

	fmt.Printf("Successfully added %d paid points to user %s\n", points, userID)
}

func getUserPoints(ctx context.Context, db *pgxpool.Pool, userID string) (free int, paid int, total int, err error) {
	query := `SELECT free_points, paid_points FROM users WHERE id = $1`

	err = db.QueryRow(ctx, query, userID).Scan(&free, &paid)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get user points: %w", err)
	}

	total = free + paid
	return free, paid, total, nil
}

func addPaidPoints(ctx context.Context, db *pgxpool.Pool, userID string, amount int) error {
	query := `
		UPDATE users 
		SET paid_points = paid_points + $1
		WHERE id = $2
	`

	_, err := db.Exec(ctx, query, amount, userID)
	if err != nil {
		return fmt.Errorf("failed to add paid points: %w", err)
	}

	log.Printf("Added %d paid points to user %s\n", amount, userID)
	return nil
}

// ==================== TRANSFER MODELS ====================

type TransferPointsRequest struct {
	ReceiverUsername string `json:"receiver_username" binding:"required"`
	Points           int    `json:"points" binding:"required,min=1"`
}

type TransferResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	PointsSent  int    `json:"points_sent"`
	FeeDeducted int    `json:"fee_deducted"`
	Received    int    `json:"received"`
	Balance     struct {
		FreePoints  int `json:"free_points"`
		PaidPoints  int `json:"paid_points"`
		TotalPoints int `json:"total_points"`
	} `json:"balance"`
}

// ==================== CALCULATE TRANSFER FEE ====================

// calculateTransferFee calculates fee: 3 points deducted for every 30 points sent
func calculateTransferFee(points int) int {
	// Fee formula: 3 points per 30 points = 10% fee
	// For every 30 points, deduct 3 points
	// Examples:
	// - 30 points → 3 point fee
	// - 60 points → 6 point fee
	// - 100 points → 10 point fee
	// - 45 points → 4.5 → 4 points (rounded down)

	fee := (points * 3) / 30
	return fee
}

// ==================== TRANSFER POINTS ENDPOINT ====================

func TransferPoints(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		senderID := c.GetString("user_id")

		var req TransferPointsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request. Please provide recipient username and points amount.",
			})
			return
		}
		if req.Points <= 30 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Minimum transfer amount is 31 points.",
			})
			return
		}

		type Sender struct {
			ID       string
			Username string
		}
		newSender := Sender{}
		err := db.QueryRow(c.Request.Context(), `SELECT  id, username FROM users WHERE id = $1`, senderID).Scan(&newSender.ID, &newSender.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get sender information",
			})
			return
		}

		// Validate recipient exists and get their ID
		var recipientID string
		var recipientUsername string
		err = db.QueryRow(c.Request.Context(), `
			SELECT id, username FROM users WHERE username = $1
		`, req.ReceiverUsername).Scan(&recipientID, &recipientUsername)

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Recipient user not found",
			})
			return
		}

		// Calculate fee (3 points per 30 sent)
		fee := calculateTransferFee(req.Points)
		totalDeduction := req.Points + fee
		pointsReceived := req.Points - fee

		fmt.Printf("Transfer calculation: Sending=%d, Fee=%d, Total deducted=%d, Received=%d\n",
			req.Points, fee, totalDeduction, pointsReceived)

		// Check sender has enough points
		senderFree, senderPaid, senderTotal, err := getUserPoints(c.Request.Context(), db, senderID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to check balance",
			})
			return
		}

		if senderTotal < totalDeduction {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Insufficient points. You have %d points but need %d (including %d fee)",
					senderTotal, totalDeduction, fee),
				"balance":  senderTotal,
				"required": totalDeduction,
				"fee":      fee,
			})
			return
		}

		// Start database transaction
		tx, err := db.Begin(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to initiate transfer",
			})
			return
		}
		defer tx.Rollback(c.Request.Context())

		// Deduct from sender (paid points first, then free)
		remaining := totalDeduction
		newSenderPaid := senderPaid
		newSenderFree := senderFree

		if senderPaid >= remaining {
			// Deduct all from paid points
			newSenderPaid = senderPaid - remaining
			remaining = 0
		} else {
			// Use all paid points, then deduct from free
			newSenderPaid = 0
			remaining -= senderPaid
			newSenderFree = senderFree - remaining
		}

		_, err = tx.Exec(c.Request.Context(), `
			UPDATE users 
			SET paid_points = $1, free_points = $2
			WHERE id = $3
		`, newSenderPaid, newSenderFree, senderID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to deduct points from sender",
			})
			return
		}

		// Add points to recipient (as free points)
		_, err = tx.Exec(c.Request.Context(), `
			UPDATE users 
			SET paid_points = paid_points + $1
			WHERE username = $2
		`, pointsReceived, req.ReceiverUsername)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to add points to recipient",
			})
			return
		}

		// Record transaction for sender
		_, err = tx.Exec(c.Request.Context(), `
			INSERT INTO transactions (
				id, user_id, type, amount, points, status, 
				from_user_id, to_user_id, created_at
			) VALUES (
				gen_random_uuid(), $1, 'transfer_sent', 0, $2, 'success', 
				$1, $3, NOW()
			)
		`, newSender.ID, -totalDeduction, recipientID)

		if err != nil {
			fmt.Printf("Failed to record sender transaction: %v\n", err)
		}

		// Record transaction for recipient
		_, err = tx.Exec(c.Request.Context(), `
			INSERT INTO transactions (
				id, user_id, type, amount, points, status, 
				from_user_id, to_user_id, created_at
			) VALUES (
				gen_random_uuid(), $1, 'transfer_received', 0, $2, 'success', 
				$3, $1, NOW()
			)
		`, recipientID, pointsReceived, newSender.ID)

		if err != nil {
			fmt.Printf("Failed to record recipient transaction: %v\n", err)
		}

		// Commit transaction
		if err = tx.Commit(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to complete transfer",
			})
			return
		}

		fmt.Printf("Transfer completed: %s → %s (%s), Amount: %d, Fee: %d, Received: %d\n",
			newSender.Username, recipientUsername, req.ReceiverUsername, req.Points, fee, pointsReceived)

		// Get updated sender balance
		free, paid, total, _ := getUserPoints(c.Request.Context(), db, senderID)

		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"message":      fmt.Sprintf("Successfully sent %d points to %s", req.Points, recipientUsername),
			"points_sent":  req.Points,
			"fee_deducted": fee,
			"received":     pointsReceived,
			"recipient":    recipientUsername,
			"balance": gin.H{
				"free_points":  free,
				"paid_points":  paid,
				"total_points": total,
			},
		})
	}
}

// ==================== GET TRANSFER HISTORY ====================

func GetTransferHistory(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		query := `
			SELECT 
				t.id,
				t.type,
				t.points,
				t.from_user_id,
				t.to_user_id,
				t.created_at,
				COALESCE(u_from.username, '') as from_username,
				COALESCE(u_to.username, '') as to_username
			FROM transactions t
			LEFT JOIN users u_from ON t.from_user_id = u_from.id
			LEFT JOIN users u_to ON t.to_user_id = u_to.id
			WHERE t.user_id = $1 
			  AND t.type IN ('transfer_sent', 'transfer_received')
			ORDER BY t.created_at DESC
			LIMIT 50
		`

		rows, err := db.Query(c.Request.Context(), query, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch transfer history",
			})
			return
		}
		defer rows.Close()

		var transfers []gin.H

		for rows.Next() {
			var (
				id           string
				txType       string
				points       int
				fromUserID   sql.NullString
				toUserID     sql.NullString
				createdAt    string
				fromUsername string
				toUsername   string
			)

			err := rows.Scan(&id, &txType, &points, &fromUserID, &toUserID,
				&createdAt, &fromUsername, &toUsername)

			if err != nil {
				continue
			}

			transfer := gin.H{
				"id":         id,
				"type":       txType,
				"points":     points,
				"created_at": createdAt,
			}

			if txType == "transfer_sent" {
				transfer["recipient"] = toUsername
				transfer["recipient_id"] = toUserID.String
			} else {
				transfer["sender"] = fromUsername
				transfer["sender_id"] = fromUserID.String
			}

			transfers = append(transfers, transfer)
		}

		c.JSON(http.StatusOK, gin.H{
			"transfers": transfers,
		})
	}
}

// ==================== CREATE TRANSACTIONS TABLE ====================

/*
CREATE TABLE IF NOT EXISTS transactions (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id),
	type VARCHAR(20) NOT NULL, -- 'purchase', 'transfer', 'deduction'
	amount INT NOT NULL, -- Amount in Naira
	points INT NOT NULL,
	reference VARCHAR(100) UNIQUE,
	status VARCHAR(20) DEFAULT 'pending', -- 'pending', 'success', 'failed'
	payment_method VARCHAR(50), -- 'paystack', 'transfer', etc.
	from_user_id UUID REFERENCES users(id), -- For transfers
	to_user_id UUID REFERENCES users(id), -- For transfers
	created_at TIMESTAMP DEFAULT NOW(),
	updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_transactions_user_id ON transactions(user_id);
CREATE INDEX idx_transactions_reference ON transactions(reference);
CREATE INDEX idx_transactions_status ON transactions(status);
*/
