package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tierguard/internal/handlers"
	"tierguard/internal/repository/postgres"
	"tierguard/internal/service"
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func main() {
	port := getEnv("PORT", "8080")
	pgURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/tierguard?sslmode=disable")
	redisURL := getEnv("REDIS_URL", "localhost:6379")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log.Println("[TierGuard] Connecting to PostgreSQL...")
	pgRepo, err := postgres.NewRepository(ctx, pgURL)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pgRepo.Close()
	log.Println("[TierGuard] Connected to PostgreSQL successfully")

	log.Println("[TierGuard] Connecting to Redis...")
	redisSvc, err := service.NewRedisService(redisURL)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisSvc.Close()
	log.Println("[TierGuard] Connected to Redis successfully")

	entitlementSvc := service.NewEntitlementService(pgRepo, redisSvc.Cache())

	// Startup cache warming: sync Postgres data into Redis
	if err := entitlementSvc.WarmUpCache(ctx); err != nil {
		log.Printf("Warning: Cache warm-up had errors: %v", err)
	}

	templatePath := getEnv("TEMPLATE_PATH", "web/templates/dashboard.html")
	adminHandler, err := handlers.NewAdminHandler(entitlementSvc, templatePath)
	if err != nil {
		log.Fatalf("Failed to load dashboard templates: %v", err)
	}

	verifyHandler := handlers.NewVerifyHandler(redisSvc)

	mux := http.NewServeMux()

	// Core API Verification endpoint
	mux.Handle("/api/verify", verifyHandler)

	// Admin UI & HTMX endpoints
	mux.HandleFunc("/admin", adminHandler.Dashboard)
	mux.HandleFunc("/admin/update-tier", adminHandler.UpdateTier)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/admin", http.StatusTemporaryRedirect)
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[TierGuard] Server listening on http://localhost:%s\n", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server listen error: %v", err)
		}
	}()

	<-stop
	log.Println("[TierGuard] Shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced shutdown: %v", err)
	}

	fmt.Println("[TierGuard] Server stopped.")
}
