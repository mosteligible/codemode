package workerclient

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mosteligible/mcp-codemode/coderunner/constants"
	"github.com/redis/go-redis/v9"
)

func TestSelectBestWorkerPrefersAvailableSlots(t *testing.T) {
	now := time.Now()
	got, ok := selectBestWorker([]WorkerCapacity{
		{
			WorkerId:       "worker-a:30031",
			State:          WorkerStateAvailable,
			CpuPercent:     5,
			MemoryPercent:  10,
			LastUpdated:    now,
			AvailableSlots: 1,
		},
		{
			WorkerId:       "worker-b:30031",
			State:          WorkerStateAvailable,
			CpuPercent:     80,
			MemoryPercent:  90,
			LastUpdated:    now,
			AvailableSlots: 4,
		},
	})
	if !ok {
		t.Fatal("selectBestWorker returned no worker")
	}
	if got.WorkerId != "worker-b:30031" {
		t.Fatalf("expected worker-b:30031, got %s", got.WorkerId)
	}
}

func TestSelectBestWorkerSkipsUnavailableWorkers(t *testing.T) {
	got, ok := selectBestWorker([]WorkerCapacity{
		{
			WorkerId:       "worker-a:30031",
			State:          WorkerStateUnavailable,
			AvailableSlots: 8,
		},
		{
			WorkerId:       "worker-b:30031",
			State:          WorkerStateAvailable,
			AvailableSlots: 2,
		},
	})
	if !ok {
		t.Fatal("selectBestWorker returned no worker")
	}
	if got.WorkerId != "worker-b:30031" {
		t.Fatalf("expected worker-b:30031, got %s", got.WorkerId)
	}
}

func TestSelectBestWorkerUsesLoadTieBreakers(t *testing.T) {
	now := time.Now()
	got, ok := selectBestWorker([]WorkerCapacity{
		{
			WorkerId:       "worker-a:30031",
			State:          WorkerStateAvailable,
			CpuPercent:     50,
			MemoryPercent:  20,
			LastUpdated:    now,
			AvailableSlots: 2,
		},
		{
			WorkerId:       "worker-b:30031",
			State:          WorkerStateAvailable,
			CpuPercent:     25,
			MemoryPercent:  80,
			LastUpdated:    now,
			AvailableSlots: 2,
		},
	})
	if !ok {
		t.Fatal("selectBestWorker returned no worker")
	}
	if got.WorkerId != "worker-b:30031" {
		t.Fatalf("expected worker-b:30031, got %s", got.WorkerId)
	}
}

func TestSelectConfiguredWorkerHostIsDeterministic(t *testing.T) {
	wc := &WorkerConnections{
		Connections: map[string]*WorkerClient{
			"worker-b:30031": nil,
			"worker-a:30031": nil,
		},
	}
	got, err := wc.selectConfiguredWorkerHost()
	if err != nil {
		t.Fatalf("selectConfiguredWorkerHost returned error: %v", err)
	}
	if got != "worker-a:30031" {
		t.Fatalf("expected worker-a:30031, got %s", got)
	}
}

func TestRedisControlPlaneRoutesAndSticksSession(t *testing.T) {
	if os.Getenv("REDIS_INTEGRATION") != "1" {
		t.Skip("set REDIS_INTEGRATION=1 to run against localhost redis")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping failed: %v", err)
	}

	sessionId := "integration-session-worker-client"
	workerA := "integration-worker-a:30031"
	workerB := "integration-worker-b:30031"
	cleanupRedisControlPlaneKeys(t, ctx, client, sessionId, workerA, workerB)
	t.Cleanup(func() {
		cleanupRedisControlPlaneKeys(t, context.Background(), client, sessionId, workerA, workerB)
	})

	putWorkerCapacity(t, ctx, client, WorkerCapacity{
		WorkerId:       workerA,
		State:          WorkerStateAvailable,
		CpuPercent:     1,
		MemoryPercent:  1,
		LastUpdated:    time.Now(),
		MaxSlots:       16,
		AvailableSlots: 3,
	})
	putWorkerCapacity(t, ctx, client, WorkerCapacity{
		WorkerId:       workerB,
		State:          WorkerStateAvailable,
		CpuPercent:     50,
		MemoryPercent:  50,
		LastUpdated:    time.Now(),
		MaxSlots:       16,
		AvailableSlots: 999,
	})

	wc := &WorkerConnections{
		Connections: make(map[string]*WorkerClient),
		redisClient: client,
		sessionTTL:  time.Minute,
	}
	if _, err := wc.GetWorker(ctx, sessionId); err != nil {
		t.Fatalf("GetWorker returned error: %v", err)
	}

	gotHost, err := client.Get(ctx, sessionWorkerKey(sessionId)).Result()
	if err != nil {
		t.Fatalf("session worker key was not written: %v", err)
	}
	if gotHost != workerB {
		t.Fatalf("expected session to route to %s, got %s", workerB, gotHost)
	}

	putWorkerCapacity(t, ctx, client, WorkerCapacity{
		WorkerId:       workerA,
		State:          WorkerStateAvailable,
		CpuPercent:     1,
		MemoryPercent:  1,
		LastUpdated:    time.Now(),
		MaxSlots:       16,
		AvailableSlots: 1000,
	})
	if _, err := wc.GetWorker(ctx, sessionId); err != nil {
		t.Fatalf("sticky GetWorker returned error: %v", err)
	}
	gotHost, err = client.Get(ctx, sessionWorkerKey(sessionId)).Result()
	if err != nil {
		t.Fatalf("session worker key was removed: %v", err)
	}
	if gotHost != workerB {
		t.Fatalf("expected session to stay on %s, got %s", workerB, gotHost)
	}
}

func putWorkerCapacity(t *testing.T, ctx context.Context, client *redis.Client, capacity WorkerCapacity) {
	t.Helper()
	rawCapacity, err := json.Marshal(capacity)
	if err != nil {
		t.Fatalf("failed to marshal capacity: %v", err)
	}
	pipe := client.Pipeline()
	pipe.SAdd(ctx, constants.RedisAvailableWorkersKey, capacity.WorkerId)
	pipe.Set(ctx, workerCapacityKey(capacity.WorkerId), rawCapacity, time.Minute)
	pipe.HSet(ctx, constants.RedisWorkerCapacitiesKey, capacity.WorkerId, rawCapacity)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("failed to seed worker capacity: %v", err)
	}
}

func cleanupRedisControlPlaneKeys(t *testing.T, ctx context.Context, client *redis.Client, sessionId string, workers ...string) {
	t.Helper()
	pipe := client.Pipeline()
	for _, worker := range workers {
		pipe.SRem(ctx, constants.RedisAvailableWorkersKey, worker)
		pipe.HDel(ctx, constants.RedisWorkerCapacitiesKey, worker)
		pipe.Del(ctx, workerCapacityKey(worker), workerSessionsKey(worker))
	}
	pipe.Del(ctx, sessionWorkerKey(sessionId))
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("failed to cleanup redis control plane keys: %v", err)
	}
}
