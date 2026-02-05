package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"webarchive/internal/ai"
	"webarchive/internal/api"
	"webarchive/internal/config"
	"webarchive/internal/db"
	"webarchive/internal/processor"
	"webarchive/internal/storage"
)

func main() {
	cfg := config.Load()

	gdb, err := db.Connect(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	store, err := storage.NewMinioStore(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOSecure, cfg.MinIOBucket)
	if err != nil {
		log.Fatalf("minio connect failed: %v", err)
	}

	proc := processor.New(store, cfg.HTTPTimeout)
	var llmClient *ai.Client
	if cfg.LLMEnabled || cfg.LLMAPIKey != "" {
		llmClient = ai.NewClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMTimeout)
	}

	r := gin.Default()
	r.Use(corsMiddleware())

	srv := &api.Server{
		DB:        gdb,
		Store:     store,
		Processor: proc,
		LLM:       llmClient,
		AutoTag:   cfg.AutoTagOnCapture,
	}
	srv.RegisterRoutes(r)

	log.Printf("listening on %s", cfg.Addr)
	if err := r.Run(cfg.Addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
