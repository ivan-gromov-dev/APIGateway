// Command grpc-health runs a standard gRPC health service or checks one through the gateway.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	address := flag.String("address", env("GRPC_HEALTH_ADDRESS", ":50051"), "gRPC listen address")
	check := flag.String("check", "", "check a gRPC health endpoint and exit")
	flag.Parse()
	if *check != "" {
		if err := checkHealth(*check); err != nil {
			log.Fatal(err)
		}
		fmt.Println("gRPC health check passed")
		return
	}

	listener, err := net.Listen("tcp", *address)
	if err != nil {
		log.Fatal(err)
	}
	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	log.Printf("gRPC health service listening on %s", *address)
	if err := server.Serve(listener); err != nil {
		log.Fatal(err)
	}
}

func checkHealth(address string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer connection.Close()
	response, err := healthpb.NewHealthClient(connection).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}
	if response.Status != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("unexpected serving status %s", response.Status)
	}
	return nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
