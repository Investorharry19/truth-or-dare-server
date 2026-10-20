package config

import (
	"log"
	"os"
)

type Config struct {
	Port              string
	DatabaseURL       string
	PaystackSecretKey string
	PaystackPublicKey string
}

func LoadConfig() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	paystackSecret := os.Getenv("PAYSTACK_SECRET_KEY")
	if paystackSecret == "" {
		log.Println("WARNING: PAYSTACK_SECRET_KEY is not set")
	}

	paystackPublic := os.Getenv("PAYSTACK_PUBLIC_KEY")
	if paystackPublic == "" {
		log.Println("WARNING: PAYSTACK_PUBLIC_KEY is not set")
	}

	return &Config{
		Port:              port,
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		PaystackSecretKey: paystackSecret,
		PaystackPublicKey: paystackPublic,
	}
}
