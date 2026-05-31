package heartbeat

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mosteligible/mcp-codemode/agent/constants"
	"github.com/redis/go-redis/v9"
)

func TestPublishWritesRedisControlPlaneKeys(t *testing.T) {
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

	workerId := "integration-agent-worker:30031"
	cleanupHeartbeatKeys(t, ctx, client, workerId)
	t.Cleanup(func() {
		cleanupHeartbeatKeys(t, context.Background(), client, workerId)
	})

	beat := &Beat{
		WorkerId:       workerId,
		interval:       5,
		containerState: fakeCapacityProvider{maxSlots: 8, availableSlots: 6},
	}
	if err := beat.publish(ctx, client); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}

	isAvailable, err := client.SIsMember(ctx, constants.RedisAvailableWorkersKey, workerId).Result()
	if err != nil {
		t.Fatalf("failed to read available worker set: %v", err)
	}
	if !isAvailable {
		t.Fatalf("expected %s in available worker set", workerId)
	}

	rawCapacity, err := client.Get(ctx, workerCapacityKey(workerId)).Bytes()
	if err != nil {
		t.Fatalf("failed to read worker capacity key: %v", err)
	}
	var capacity WorkerCapacity
	if err := json.Unmarshal(rawCapacity, &capacity); err != nil {
		t.Fatalf("failed to unmarshal worker capacity: %v", err)
	}
	if capacity.WorkerId != workerId {
		t.Fatalf("expected worker id %s, got %s", workerId, capacity.WorkerId)
	}
	if capacity.MaxSlots != 8 || capacity.AvailableSlots != 6 {
		t.Fatalf("expected capacity 8/6, got %d/%d", capacity.MaxSlots, capacity.AvailableSlots)
	}

	if _, err := client.HGet(ctx, constants.RedisWorkerCapacitiesKey, workerId).Bytes(); err != nil {
		t.Fatalf("failed to read worker capacity hash mirror: %v", err)
	}
}

func TestRemoveDeletesRedisControlPlaneKeys(t *testing.T) {
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

	workerId := "integration-agent-remove-worker:30031"
	cleanupHeartbeatKeys(t, ctx, client, workerId)
	beat := &Beat{
		WorkerId:       workerId,
		interval:       5,
		containerState: fakeCapacityProvider{maxSlots: 4, availableSlots: 2},
	}
	if err := beat.publish(ctx, client); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
	if err := beat.remove(ctx, client); err != nil {
		t.Fatalf("remove returned error: %v", err)
	}

	isAvailable, err := client.SIsMember(ctx, constants.RedisAvailableWorkersKey, workerId).Result()
	if err != nil {
		t.Fatalf("failed to read available worker set: %v", err)
	}
	if isAvailable {
		t.Fatalf("expected %s to be removed from available worker set", workerId)
	}
	if err := client.Get(ctx, workerCapacityKey(workerId)).Err(); err != redis.Nil {
		t.Fatalf("expected capacity key to be removed, got %v", err)
	}
	if err := client.HGet(ctx, constants.RedisWorkerCapacitiesKey, workerId).Err(); err != redis.Nil {
		t.Fatalf("expected capacity hash entry to be removed, got %v", err)
	}
}

type fakeCapacityProvider struct {
	maxSlots       int
	availableSlots int
}

func (f fakeCapacityProvider) GetMaxSlots() int {
	return f.maxSlots
}

func (f fakeCapacityProvider) GetAvailableSlots() int {
	return f.availableSlots
}

func cleanupHeartbeatKeys(t *testing.T, ctx context.Context, client *redis.Client, workerId string) {
	t.Helper()
	pipe := client.Pipeline()
	pipe.SRem(ctx, constants.RedisAvailableWorkersKey, workerId)
	pipe.HDel(ctx, constants.RedisWorkerCapacitiesKey, workerId)
	pipe.Del(ctx, workerCapacityKey(workerId), workerHeartbeatKey(workerId))
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("failed to cleanup heartbeat keys: %v", err)
	}
}
