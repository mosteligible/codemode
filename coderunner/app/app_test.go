package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/mosteligible/mcp-codemode/agent-proto/pb"
	"github.com/mosteligible/mcp-codemode/coderunner/config"
	"github.com/mosteligible/mcp-codemode/coderunner/constants"
	workerclient "github.com/mosteligible/mcp-codemode/coderunner/core/worker_client"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestRunCodeRoutesThroughRedisSelectedWorker(t *testing.T) {
	if os.Getenv("REDIS_INTEGRATION") != "1" {
		t.Skip("set REDIS_INTEGRATION=1 to run against localhost redis")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	t.Cleanup(func() { _ = redisClient.Close() })
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping failed: %v", err)
	}

	workerHost := "integration-app-worker:30031"
	sessionId := "integration-app-session"
	cleanupRedisKeys(t, ctx, redisClient, workerHost, sessionId)
	t.Cleanup(func() {
		cleanupRedisKeys(t, context.Background(), redisClient, workerHost, sessionId)
	})
	seedWorkerCapacity(t, ctx, redisClient, workerclient.WorkerCapacity{
		WorkerId:       workerHost,
		State:          workerclient.WorkerStateAvailable,
		CpuPercent:     1,
		MemoryPercent:  1,
		LastUpdated:    time.Now(),
		MaxSlots:       8,
		AvailableSlots: 8,
	})

	fakeAgent := &fakeAgentClient{}
	workerClients := workerclient.NewWorkerConnections(&config.Config{}, redisClient)
	workerClients.AddClientForHost(workerHost, &workerclient.WorkerClient{Client: fakeAgent})

	app := &App{workerClients: workerClients}
	body := bytes.NewBufferString(`{"code":"print('hello')","language":"python","sessionId":"` + sessionId + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/run", body)
	rec := httptest.NewRecorder()

	app.RunCode(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	if fakeAgent.executeCodeRequest == nil {
		t.Fatal("expected ExecuteCode to be called")
	}
	if fakeAgent.executeCodeRequest.SessionId != sessionId {
		t.Fatalf("expected session id %s, got %s", sessionId, fakeAgent.executeCodeRequest.SessionId)
	}
	if fakeAgent.executeCodeRequest.Language != "python" {
		t.Fatalf("expected language python, got %s", fakeAgent.executeCodeRequest.Language)
	}

	gotHost, err := redisClient.Get(ctx, constants.RedisSessionWorkerPrefix+sessionId).Result()
	if err != nil {
		t.Fatalf("session worker key was not written: %v", err)
	}
	if gotHost != workerHost {
		t.Fatalf("expected session to route to %s, got %s", workerHost, gotHost)
	}
}

type fakeAgentClient struct {
	executeCodeRequest *pb.ExecuteCodeRequest
}

func (f *fakeAgentClient) Status(context.Context, *emptypb.Empty, ...grpc.CallOption) (*pb.HealthStatus, error) {
	return &pb.HealthStatus{Code: 0, Message: "healthy"}, nil
}

func (f *fakeAgentClient) ExecuteCode(_ context.Context, req *pb.ExecuteCodeRequest, _ ...grpc.CallOption) (*pb.ExecuteCodeResponse, error) {
	f.executeCodeRequest = req
	return &pb.ExecuteCodeResponse{ExitCode: 0, Output: "hello\n"}, nil
}

func (f *fakeAgentClient) ExecuteCodeFresh(_ context.Context, req *pb.ExecuteCodeRequest, _ ...grpc.CallOption) (*pb.ExecuteCodeResponse, error) {
	f.executeCodeRequest = req
	return &pb.ExecuteCodeResponse{ExitCode: 0, Output: "hello\n"}, nil
}

func seedWorkerCapacity(t *testing.T, ctx context.Context, client *redis.Client, capacity workerclient.WorkerCapacity) {
	t.Helper()
	rawCapacity, err := json.Marshal(capacity)
	if err != nil {
		t.Fatalf("failed to marshal worker capacity: %v", err)
	}
	pipe := client.Pipeline()
	pipe.SAdd(ctx, constants.RedisAvailableWorkersKey, capacity.WorkerId)
	pipe.Set(ctx, constants.RedisWorkerCapacityPrefix+capacity.WorkerId, rawCapacity, time.Minute)
	pipe.HSet(ctx, constants.RedisWorkerCapacitiesKey, capacity.WorkerId, rawCapacity)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("failed to seed worker capacity: %v", err)
	}
}

func cleanupRedisKeys(t *testing.T, ctx context.Context, client *redis.Client, workerHost, sessionId string) {
	t.Helper()
	pipe := client.Pipeline()
	pipe.SRem(ctx, constants.RedisAvailableWorkersKey, workerHost)
	pipe.HDel(ctx, constants.RedisWorkerCapacitiesKey, workerHost)
	pipe.Del(ctx, constants.RedisWorkerCapacityPrefix+workerHost)
	pipe.Del(ctx, constants.RedisSessionWorkerPrefix+sessionId)
	pipe.Del(ctx, constants.RedisWorkerSessionsPrefix+workerHost)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("failed to cleanup redis keys: %v", err)
	}
}
