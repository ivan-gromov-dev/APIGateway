package integration

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/ivan-gromov-dev/APIGateway/internal/config"
	"github.com/ivan-gromov-dev/APIGateway/test/integration/internal/testenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestGRPCProxyAcceptsGRPCGoClientOverH2C(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	backend := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(backend, healthServer)
	go func() { _ = backend.Serve(listener) }()
	t.Cleanup(func() { backend.GracefulStop() })

	environment := testenv.New(t, testenv.WithGRPC(config.GRPC{Enabled: true, Routes: []config.GRPCRoute{{
		PathPrefix: "/grpc.health.v1.Health/", Upstream: "http://" + listener.Addr().String(),
	}}}))
	connection, err := grpc.NewClient(environment.PublicAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := healthpb.NewHealthClient(connection).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("status=%s", response.Status)
	}
}
