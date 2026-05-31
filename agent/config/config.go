package config

import (
	"github.com/mosteligible/mcp-codemode/agent/constants"
	"github.com/mosteligible/mcp-codemode/agent/core/common"
)

type Config struct {
	DockerApiVersion             string
	DockerImageName              string
	WorkerHost                   string
	WorkerAddress                string
	WorkerPort                   string
	MinActiveContainers          int
	MaxActiveContainers          int
	ActiveContainerCheckInterval int
	HeartBeatInterval            int

	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDb       int
}

func NewConfig() *Config {
	workerHost := common.GetEnvironmentVariable("WORKER_HOST", "localhost")
	if workerHost == "" {
		workerHost = "localhost"
	}
	workerPort := common.GetEnvironmentVariable("WORKER_PORT", constants.DefaultWorkerPort)
	if workerPort == "" {
		workerPort = constants.DefaultWorkerPort
	}
	workerAddress := common.GetEnvironmentVariable("WORKER_ADDRESS", "")
	if workerAddress == "" {
		workerAddress = normalizeWorkerAddress(workerHost, workerPort)
	}

	return &Config{
		DockerApiVersion:             common.GetEnvironmentVariable("DOCKER_API_VERSION", constants.DefaultDockerApiVersion),
		DockerImageName:              common.GetEnvironmentVariable("DOCKER_IMAGE_NAME", constants.DefaultDockerImageName),
		WorkerHost:                   workerHost,
		WorkerAddress:                workerAddress,
		WorkerPort:                   workerPort,
		MinActiveContainers:          common.GetEnvironmentVariable("MIN_ACTIVE_CONTAINERS", constants.DefaultMinActive),
		MaxActiveContainers:          common.GetEnvironmentVariable("MAX_ACTIVE_CONTAINERS", constants.DefaultMaxActive),
		ActiveContainerCheckInterval: common.GetEnvironmentVariable("ACTIVE_CONTAINER_CHECK_INTERVAL", constants.DefaultContainerCheckInterval),
		HeartBeatInterval:            common.GetEnvironmentVariable("HEART_BEAT_INTERVAL", constants.DefaultHeartBeatInterval),
		RedisHost:                    common.GetEnvironmentVariable("REDIS_HOST", "localhost"),
		RedisPort:                    common.GetEnvironmentVariable("REDIS_PORT", "6379"),
		RedisPassword:                common.GetEnvironmentVariable("REDIS_PASSWORD", ""),
		RedisDb:                      common.GetEnvironmentVariable("REDIS_DB", 0),
	}
}

func normalizeWorkerAddress(host, port string) string {
	if port == "" {
		return host
	}
	if port[0] == ':' {
		return host + port
	}
	return host + ":" + port
}
