package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"order-service/internal/db"
	"order-service/internal/httpapi"
	"order-service/internal/logclient"
	"order-service/internal/orders"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func main() {
	port := getEnvInt("PORT", 3002)
	databaseURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/order_service")
	logServiceGRPCURL := getEnv("LOG_SERVICE_GRPC_URL", "localhost:50051")

	ctx := context.Background()

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Failed to create database pool: %v", err)
	}

	if err := db.EnsureOrdersTable(ctx, pool); err != nil {
		log.Fatalf("Failed to ensure orders table: %v", err)
	}

	repo := orders.NewRepository(pool)

	logClient, err := logclient.New(logServiceGRPCURL)
	if err != nil {
		log.Fatalf("Failed to create log client: %v", err)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: httpapi.NewHandler(repo, logClient),
	}

	go func() {
		log.Printf("order-service listening on port %d", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = httpServer.Shutdown(shutdownCtx)
	_ = logClient.Close()
	pool.Close()
}
