package jwt

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var accessSecret = []byte(os.Getenv("JWT_SECRET"))
var refreshSecret = []byte(os.Getenv("JWT_REFRESH_SECRET"))

func init() {
	if len(refreshSecret) == 0 {
		refreshSecret = accessSecret
	}
}

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	jwt.RegisteredClaims
}

func Generate(userID uuid.UUID) (string, error) {
	return GenerateAccessToken(userID)
}

func GenerateAccessToken(userID uuid.UUID) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(accessSecret)
}

func GenerateRefreshToken(userID uuid.UUID) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(refreshSecret)
}

func GenerateTokens(userID uuid.UUID) (string, string, error) {
	accessToken, err := GenerateAccessToken(userID)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := GenerateRefreshToken(userID)
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

func ValidateAccessToken(tokenString string) (string, error) {
	return validateToken(tokenString, accessSecret)
}

func ValidateRefreshToken(tokenString string) (string, error) {
	return validateToken(tokenString, refreshSecret)
}

func validateToken(tokenString string, secret []byte) (string, error) {
	if tokenString == "" {
		return "", errors.New("token is empty")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})

	if err != nil {
		fmt.Printf("  Token validation error: %v\n", err)
		return "", err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		fmt.Printf("  Token valid for user: %s\n", claims.UserID)
		return claims.UserID.String(), nil
	}

	fmt.Printf("  Invalid token claims\n")
	return "", errors.New("invalid token")
}
