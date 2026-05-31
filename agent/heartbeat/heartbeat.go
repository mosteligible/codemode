package heartbeat

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/mosteligible/mcp-codemode/agent/config"
	"github.com/mosteligible/mcp-codemode/agent/constants"
	"github.com/mosteligible/mcp-codemode/agent/core/common"
	"github.com/mosteligible/mcp-codemode/agent/states"
	"github.com/redis/go-redis/v9"
)

type WorkerState string

const (
	WorkerStateAvailable   WorkerState = "available"
	WorkerStateUnavailable WorkerState = "unavailable"
)

type Beat struct {
	WorkerId       string    `json:"worker_id"`
	LastUpdated    time.Time `json:"last_updated"`
	interval       int
	containerState capacityProvider
}

type WorkerCapacity struct {
	WorkerId       string      `json:"worker_id"`
	State          WorkerState `json:"state"`
	CpuPercent     float64     `json:"cpu_percent"`
	MemoryPercent  float64     `json:"memory_percent"`
	LastUpdated    time.Time   `json:"last_updated"`
	MaxSlots       int         `json:"max_slots"`
	AvailableSlots int         `json:"available_slots"`
}

type capacityProvider interface {
	GetMaxSlots() int
	GetAvailableSlots() int
}

func (b *Beat) MarshalBinary() ([]byte, error) {
	return json.Marshal(b)
}

func (b *Beat) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, b)
}

func (wc *WorkerCapacity) MarshalBinary() ([]byte, error) {
	return json.Marshal(wc)
}

func (wc *WorkerCapacity) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, wc)
}

func NewBeat(appConfig *config.Config, containerState *states.ContainerState) *Beat {
	return &Beat{
		WorkerId:       appConfig.WorkerAddress,
		interval:       appConfig.HeartBeatInterval,
		containerState: containerState,
	}
}

func (b *Beat) Start(redisClient *redis.Client, shutdownSignal chan struct{}) {
	ticker := time.NewTicker(time.Duration(b.interval) * time.Second)
	defer ticker.Stop()

	if err := b.publish(context.Background(), redisClient); err != nil {
		slog.Error("error publishing initial worker heartbeat", "workerId", b.WorkerId, "error", err.Error())
	}

	for {
		select {
		case <-ticker.C:
			if err := b.publish(context.Background(), redisClient); err != nil {
				slog.Error("error publishing worker heartbeat", "workerId", b.WorkerId, "error", err.Error())
			}
		case <-shutdownSignal:
			if err := b.remove(context.Background(), redisClient); err != nil {
				slog.Error("error removing worker heartbeat", "workerId", b.WorkerId, "error", err.Error())
			}
			return
		}
	}
}

func (b *Beat) publish(ctx context.Context, redisClient *redis.Client) error {
	workerCapacity, err := GetWorkerCapacity(b.WorkerId, b.containerState)
	if err != nil {
		return err
	}

	now := time.Now()
	b.LastUpdated = now
	workerCapacity.LastUpdated = now

	capacityBytes, err := json.Marshal(workerCapacity)
	if err != nil {
		return err
	}

	ttl := b.heartbeatTTL()
	pipe := redisClient.Pipeline()
	pipe.SAdd(ctx, constants.RedisAvailableWorkersKey, b.WorkerId)
	pipe.Set(ctx, workerHeartbeatKey(b.WorkerId), b, ttl)
	pipe.Set(ctx, workerCapacityKey(b.WorkerId), workerCapacity, ttl)
	pipe.HSet(ctx, constants.RedisWorkerCapacitiesKey, b.WorkerId, capacityBytes)
	_, err = pipe.Exec(ctx)
	return err
}

func (b *Beat) remove(ctx context.Context, redisClient *redis.Client) error {
	pipe := redisClient.Pipeline()
	pipe.SRem(ctx, constants.RedisAvailableWorkersKey, b.WorkerId)
	pipe.HDel(ctx, constants.RedisWorkerCapacitiesKey, b.WorkerId)
	pipe.Del(ctx, workerHeartbeatKey(b.WorkerId), workerCapacityKey(b.WorkerId))
	_, err := pipe.Exec(ctx)
	return err
}

func (b *Beat) heartbeatTTL() time.Duration {
	ttl := b.interval * 3
	if ttl < 15 {
		ttl = 15
	}
	return time.Duration(ttl) * time.Second
}

func workerHeartbeatKey(workerId string) string {
	return constants.RedisWorkerHeartbeatPrefix + workerId
}

func workerCapacityKey(workerId string) string {
	return constants.RedisWorkerCapacityPrefix + workerId
}

func GetWorkerCapacity(workerId string, containerState capacityProvider) (*WorkerCapacity, error) {
	// returns the current cpu and memory usage of the worker, as well as the number of available slots for new tasks
	hostResourceUsage, err := common.GetHostResourceUsage()
	if err != nil {
		return nil, err
	}

	return &WorkerCapacity{
		WorkerId:       workerId,
		State:          WorkerStateAvailable,
		CpuPercent:     hostResourceUsage.CPUPercent,
		MemoryPercent:  hostResourceUsage.MemoryPercent,
		LastUpdated:    time.Now(),
		MaxSlots:       containerState.GetMaxSlots(),
		AvailableSlots: containerState.GetAvailableSlots(),
	}, nil
}
