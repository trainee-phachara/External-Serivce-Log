package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"external-service-log/internal/buffer"
	"external-service-log/internal/flusher"
	pb "external-service-log/internal/grpc/pb"
	"external-service-log/internal/grpcserver"
	"external-service-log/internal/httpapi"
	"external-service-log/internal/mongostore"
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
	port := getEnvInt("PORT", 3000)
	grpcPort := getEnvInt("GRPC_PORT", 50051)
	mongoURI := getEnv("MONGO_URI", "mongodb://localhost:27017")
	mongoDBName := getEnv("MONGO_DB_NAME", "service_logs")
	flushMaxSize := getEnvInt("FLUSH_MAX_SIZE", 100)
	flushIntervalMS := getEnvInt("FLUSH_INTERVAL_MS", 5000)

	ctx := context.Background()

	store, err := mongostore.Connect(ctx, mongoURI, mongoDBName)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	buf := buffer.New()
	fl := flusher.New(buf, store.InsertLogs, flusher.Options{
		MaxSize:  flushMaxSize,
		Interval: time.Duration(flushIntervalMS) * time.Millisecond,
	})
	fl.Start()

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: httpapi.NewHandler(buf, fl),
	}

	go func() {
		log.Printf("external-service-log HTTP ingest listening on port %d", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		log.Fatalf("Failed to listen on gRPC port %d: %v", grpcPort, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterIngestServiceServer(grpcServer, grpcserver.New(buf, fl))

	go func() {
		log.Printf("external-service-log gRPC ingest listening on port %d", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = httpServer.Shutdown(shutdownCtx)
	grpcServer.GracefulStop()
	fl.Stop()
	_ = fl.Flush(shutdownCtx)
	_ = store.Close(shutdownCtx)
}
