package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Investorharry19/truth-or-dare-server/cronjobs"
	"github.com/Investorharry19/truth-or-dare-server/internal/auth"
	"github.com/Investorharry19/truth-or-dare-server/internal/payments"
	"github.com/Investorharry19/truth-or-dare-server/internal/room"
	"github.com/Investorharry19/truth-or-dare-server/internal/user"
	"github.com/Investorharry19/truth-or-dare-server/middleware"
	"github.com/Investorharry19/truth-or-dare-server/pkg/db"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	db.Connect()
	defer db.Pool.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	payments.PAYSTACK_SECRET_KEY = os.Getenv("PAYSTACK_SECRET_KEY")
	payments.PAYSTACK_PUBLIC_KEY = os.Getenv("PAYSTACK_PUBLIC_KEY")

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://127.0.0.1:3000", "https://vivid-chaos-web.vercel.app", "https://tod.amehhendrix.online"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Content-Type", "Authorization", "X-Requested-With"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", openAPISpec)
	})
	r.GET("/docs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerUIHTML))
	})

	// Catch all OPTIONS preflight requests
	r.OPTIONS("/*path", func(c *gin.Context) {
		c.AbortWithStatus(204)
	})

	userRepo := user.NewRepository()
	authService := auth.NewService(userRepo)
	authHandler := auth.NewHandler(authService)

	authRoutes := r.Group("/auth")
	{
		authRoutes.POST("/register", authHandler.Register)
		authRoutes.POST("/login", authHandler.Login)
		authRoutes.POST("/google", authHandler.Google)
		authRoutes.POST("/refresh", authHandler.Refresh)
		authRoutes.POST("/forgot-password", authHandler.ForgotPassword)
		authRoutes.POST("/reset-password", authHandler.ResetPassword)
		authRoutes.POST("/verify-email", authHandler.VerifyEmail)
		authRoutes.GET("/verify-email", authHandler.VerifyEmail)
		authRoutes.POST("/resend-email-verification-token", authHandler.ResendVerification)
		authRoutes.POST("/logout", middleware.AuthMiddleware(), authHandler.Logout)
		authRoutes.GET("/me", middleware.AuthMiddleware(), authHandler.MeHandler)
		authRoutes.GET("/points", middleware.AuthMiddleware(), authHandler.PointsHandler)
		authRoutes.GET("/streak", middleware.AuthMiddleware(), authHandler.StreakGetHandler)
		authRoutes.POST("/streak", middleware.AuthMiddleware(), authHandler.StreakRecordHandler)
	}
	roomRoutes := r.Group("/room")
	{

		// Keep auth for regular HTTP routes
		roomRoutes.POST("/create", middleware.AuthMiddleware(), room.CreateRoomHandler)
		roomRoutes.POST("/lobbies/:id/claim", middleware.AuthMiddleware(), room.ClaimLobbyHandler)
		roomRoutes.GET("/:id", middleware.AuthMiddleware(), room.GetRoomHandler)
		roomRoutes.POST("/:id/upload", middleware.AuthMiddleware(), room.UploadFileHandler)

	}

	payment := r.Group("/payment")
	{
		// Protected routes (require authentication)
		payment.POST("/initialize", middleware.AuthMiddleware(), payments.InitializePayment(db.Pool))
		payment.GET("/verify/:reference", middleware.AuthMiddleware(), payments.VerifyPayment(db.Pool))
		payment.GET("/callback", payments.PaymentCallbackHandler())

		// Webhook (no auth - verified by signature)
		payment.POST("/webhook", payments.PaystackWebhook(db.Pool))
	}

	points := r.Group("/points")
	{
		points.POST("/transfer", middleware.AuthMiddleware(), payments.TransferPoints(db.Pool))
		points.GET("/transfers", middleware.AuthMiddleware(), payments.GetTransferHistory(db.Pool))
	}

	r.GET("/", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{
			"message":     "Welcome to the Truth or Dare Game Server!",
			"last_update": "2026-07-06",
		})
	})

	//Print all registered routes BEFORE starting server
	fmt.Println("\n🔍 Registered Routes:")
	for _, route := range r.Routes() {
		fmt.Printf("  %s\t%s\n", route.Method, route.Path)
	}
	fmt.Println()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "okay",
			"db":     "connected",
		})
	})
	r.GET("/ws", room.ServeWs)

	// Initialize LiveKit helper routes (token issuance)
	room.InitializeLiveKitRoutes(r)

	cronjobs.StartPointsResetCron(db.Pool)
	room.InitializeLobbyPool()
	room.StartInactivityCleanup(room.InactivityTimeout)
	log.Println("Server running on port", port)
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

//go:embed openapi.yaml
var openAPISpec []byte

const swaggerUIHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Truth or Dare API docs</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css">
    <style>body { margin: 0; }</style>
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
    <script>
      window.onload = () => SwaggerUIBundle({
        url: "/openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true
      });
    </script>
  </body>
</html>`
