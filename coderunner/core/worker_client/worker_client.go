package workerclient

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	pb "github.com/mosteligible/mcp-codemode/agent-proto/pb"
	"github.com/mosteligible/mcp-codemode/coderunner/config"
	"github.com/mosteligible/mcp-codemode/coderunner/constants"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type WorkerClient struct {
	conn   *grpc.ClientConn
	Client pb.AgentClient
}

func NewWorkerClient(address string) (*WorkerClient, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	), grpc.WithStatsHandler(
		otelgrpc.NewClientHandler(),
	))
	if err != nil {
		return nil, err
	}
	client := pb.NewAgentClient(conn)
	return &WorkerClient{
		conn:   conn,
		Client: client,
	}, nil
}

type WorkerState string

const (
	WorkerStateAvailable   WorkerState = "available"
	WorkerStateUnavailable WorkerState = "unavailable"

	defaultSessionTTL = 4 * time.Hour
)

type WorkerCapacity struct {
	WorkerId       string      `json:"worker_id"`
	State          WorkerState `json:"state"`
	CpuPercent     float64     `json:"cpu_percent"`
	MemoryPercent  float64     `json:"memory_percent"`
	LastUpdated    time.Time   `json:"last_updated"`
	MaxSlots       int         `json:"max_slots"`
	AvailableSlots int         `json:"available_slots"`
}

type WorkerConnections struct {
	Connections map[string]*WorkerClient
	redisClient *redis.Client
	sessionTTL  time.Duration
	lock        sync.RWMutex
}

func NewWorkerConnections(conf *config.Config, redisClient *redis.Client) *WorkerConnections {
	wc := &WorkerConnections{
		Connections: make(map[string]*WorkerClient),
		redisClient: redisClient,
		sessionTTL:  defaultSessionTTL,
	}
	for _, host := range conf.RemoteHosts {
		if _, err := wc.connectClientForHost(host); err != nil {
			slog.Error("could not create worker client", "host", host, "error", err.Error())
		}
	}
	return wc
}

func (wc *WorkerConnections) GetWorker(ctx context.Context, sessionId string) (*WorkerClient, error) {
	sessionId = strings.TrimSpace(sessionId)
	if sessionId != "" {
		host, err := wc.getSessionWorkerHost(ctx, sessionId)
		if err == nil && host != "" {
			if wc.isWorkerAlive(ctx, host) {
				return wc.connectClientForHost(host)
			}
			slog.Warn("clearing session mapping for stale worker", "sessionId", sessionId, "host", host)
			wc.RemoveUserSessionFromHost(ctx, host, sessionId, nil)
		} else if err != nil && !errors.Is(err, redis.Nil) {
			slog.Warn("failed to load session worker from redis", "sessionId", sessionId, "error", err.Error())
		}
	}

	host, err := wc.selectWorkerHost(ctx)
	if err != nil {
		return nil, err
	}
	if sessionId != "" {
		if err := wc.AddUserSessionToHost(ctx, host, sessionId, nil); err != nil {
			slog.Warn("failed to persist session worker mapping", "sessionId", sessionId, "host", host, "error", err.Error())
		}
	}
	return wc.connectClientForHost(host)
}

func (wc *WorkerConnections) GetWorkerForSession(ctx context.Context, sessionId string) (*WorkerClient, error) {
	return wc.GetWorker(ctx, sessionId)
}

func (wc *WorkerConnections) getSessionWorkerHost(ctx context.Context, sessionId string) (string, error) {
	if wc.redisClient == nil {
		return "", redis.Nil
	}
	return wc.redisClient.Get(ctx, sessionWorkerKey(sessionId)).Result()
}

func (wc *WorkerConnections) selectWorkerHost(ctx context.Context) (string, error) {
	if wc.redisClient != nil {
		host, err := wc.selectWorkerHostFromRedis(ctx)
		if err == nil {
			return host, nil
		}
		if !errors.Is(err, redis.Nil) {
			slog.Warn("failed to select worker from redis", "error", err.Error())
		}
	}
	return wc.selectConfiguredWorkerHost()
}

func (wc *WorkerConnections) selectWorkerHostFromRedis(ctx context.Context) (string, error) {
	hosts, err := wc.redisClient.SMembers(ctx, constants.RedisAvailableWorkersKey).Result()
	if err != nil {
		return "", err
	}
	if len(hosts) == 0 {
		return "", redis.Nil
	}

	capacities := make([]WorkerCapacity, 0, len(hosts))
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		capacity, err := wc.getWorkerCapacity(ctx, host)
		if errors.Is(err, redis.Nil) {
			wc.removeStaleWorker(ctx, host)
			continue
		}
		if err != nil {
			slog.Warn("failed to load worker capacity", "host", host, "error", err.Error())
			continue
		}
		if capacity.WorkerId == "" {
			capacity.WorkerId = host
		}
		capacities = append(capacities, capacity)
	}

	selected, ok := selectBestWorker(capacities)
	if !ok {
		return "", redis.Nil
	}
	return selected.WorkerId, nil
}

func (wc *WorkerConnections) getWorkerCapacity(ctx context.Context, host string) (WorkerCapacity, error) {
	rawCapacity, err := wc.redisClient.Get(ctx, workerCapacityKey(host)).Bytes()
	if err != nil {
		return WorkerCapacity{}, err
	}
	var capacity WorkerCapacity
	if err := json.Unmarshal(rawCapacity, &capacity); err != nil {
		return WorkerCapacity{}, err
	}
	return capacity, nil
}

func (wc *WorkerConnections) isWorkerAlive(ctx context.Context, host string) bool {
	if wc.redisClient == nil {
		_, ok := wc.GetClientForHost(host)
		return ok
	}
	capacity, err := wc.getWorkerCapacity(ctx, host)
	if err != nil {
		return false
	}
	return capacity.State != WorkerStateUnavailable
}

func (wc *WorkerConnections) removeStaleWorker(ctx context.Context, host string) {
	pipe := wc.redisClient.Pipeline()
	pipe.SRem(ctx, constants.RedisAvailableWorkersKey, host)
	pipe.HDel(ctx, constants.RedisWorkerCapacitiesKey, host)
	_, _ = pipe.Exec(ctx)
}

func (wc *WorkerConnections) selectConfiguredWorkerHost() (string, error) {
	wc.lock.RLock()
	defer wc.lock.RUnlock()
	if len(wc.Connections) == 0 {
		return "", errors.New("no available worker connections")
	}
	hosts := make([]string, 0, len(wc.Connections))
	for host := range wc.Connections {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts[0], nil
}

func selectBestWorker(capacities []WorkerCapacity) (WorkerCapacity, bool) {
	filtered := make([]WorkerCapacity, 0, len(capacities))
	for _, capacity := range capacities {
		if strings.TrimSpace(capacity.WorkerId) == "" {
			continue
		}
		if capacity.State == WorkerStateUnavailable {
			continue
		}
		filtered = append(filtered, capacity)
	}
	if len(filtered) == 0 {
		return WorkerCapacity{}, false
	}

	sort.Slice(filtered, func(i, j int) bool {
		left := filtered[i]
		right := filtered[j]
		if left.AvailableSlots != right.AvailableSlots {
			return left.AvailableSlots > right.AvailableSlots
		}
		if left.CpuPercent != right.CpuPercent {
			return left.CpuPercent < right.CpuPercent
		}
		if left.MemoryPercent != right.MemoryPercent {
			return left.MemoryPercent < right.MemoryPercent
		}
		if !left.LastUpdated.Equal(right.LastUpdated) {
			return left.LastUpdated.After(right.LastUpdated)
		}
		return left.WorkerId < right.WorkerId
	})

	return filtered[0], true
}

func (wc *WorkerConnections) GetClientForHost(host string) (*WorkerClient, bool) {
	wc.lock.RLock()
	defer wc.lock.RUnlock()
	client, ok := wc.Connections[host]
	return client, ok
}

func (wc *WorkerConnections) CloseAll() {
	wc.lock.RLock()
	defer wc.lock.RUnlock()
	for _, client := range wc.Connections {
		if client != nil && client.conn != nil {
			client.conn.Close()
		}
	}
}

func (wc *WorkerConnections) AddClientForHost(host string, client *WorkerClient) {
	wc.lock.Lock()
	defer wc.lock.Unlock()
	wc.Connections[host] = client
}

func (wc *WorkerConnections) connectClientForHost(host string) (*WorkerClient, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("worker host cannot be empty")
	}
	if client, ok := wc.GetClientForHost(host); ok {
		return client, nil
	}
	client, err := NewWorkerClient(host)
	if err != nil {
		return nil, err
	}
	wc.AddClientForHost(host, client)
	return client, nil
}

func (wc *WorkerConnections) RemoveWorkerHost(ctx context.Context, host string, redisClient *redis.Client) {
	wc.lock.Lock()
	client, ok := wc.Connections[host]
	if ok && client != nil && client.conn != nil {
		client.conn.Close()
	}
	delete(wc.Connections, host)
	wc.lock.Unlock()

	if redisClient == nil {
		redisClient = wc.redisClient
	}
	if redisClient == nil {
		return
	}

	sessions, _ := redisClient.SMembers(ctx, workerSessionsKey(host)).Result()
	pipe := redisClient.Pipeline()
	pipe.SRem(ctx, constants.RedisAvailableWorkersKey, host)
	pipe.HDel(ctx, constants.RedisWorkerCapacitiesKey, host)
	pipe.Del(ctx, workerCapacityKey(host), workerSessionsKey(host))
	for _, sessionId := range sessions {
		pipe.Del(ctx, sessionWorkerKey(sessionId))
	}
	_, _ = pipe.Exec(ctx)
}

func (wc *WorkerConnections) RemoveUserSessionFromHost(ctx context.Context, host, sessionId string, redisClient *redis.Client) {
	if redisClient == nil {
		redisClient = wc.redisClient
	}
	if redisClient == nil {
		return
	}

	pipe := redisClient.Pipeline()
	pipe.Del(ctx, sessionWorkerKey(sessionId))
	pipe.SRem(ctx, workerSessionsKey(host), sessionId)
	_, _ = pipe.Exec(ctx)
}

func (wc *WorkerConnections) AddUserSessionToHost(ctx context.Context, host, sessionId string, redisClient *redis.Client) error {
	if redisClient == nil {
		redisClient = wc.redisClient
	}
	if redisClient == nil {
		return nil
	}

	pipe := redisClient.Pipeline()
	pipe.Set(ctx, sessionWorkerKey(sessionId), host, wc.sessionTTL)
	pipe.SAdd(ctx, workerSessionsKey(host), sessionId)
	pipe.Expire(ctx, workerSessionsKey(host), wc.sessionTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func sessionWorkerKey(sessionId string) string {
	return constants.RedisSessionWorkerPrefix + sessionId
}

func workerCapacityKey(host string) string {
	return constants.RedisWorkerCapacityPrefix + host
}

func workerSessionsKey(host string) string {
	return constants.RedisWorkerSessionsPrefix + host
}
