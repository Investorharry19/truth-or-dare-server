package room

import (
	"github.com/Investorharry19/truth-or-dare-server/internal/livekit"
	"github.com/Investorharry19/truth-or-dare-server/middleware"
	"github.com/gin-gonic/gin"
)

// InitializeLiveKitRoutes sets up LiveKit routes
func InitializeLiveKitRoutes(r *gin.Engine) {
	tokenConfig := livekit.NewTokenConfig()
	tokenHandler := livekit.NewTokenHandler(tokenConfig)

	lkRoutes := r.Group("/livekit")
	lkRoutes.Use(middleware.AuthMiddleware())
	{
		lkRoutes.POST("/token", tokenHandler.GetToken)
		// Alternative endpoint that validates token in query
		lkRoutes.GET("/token", tokenHandler.GetTokenWithAuth)
	}
}
