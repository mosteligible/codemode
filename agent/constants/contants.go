package constants

const (
	DefaultDockerApiVersion       = "1.40"
	DefaultDockerImageName        = "python:3.14-slim"
	DefaultWorkerPort             = ":30031"
	DefaultMinActive              = 2
	DefaultMaxActive              = 8
	DefaultContainerCheckInterval = 30 // seconds

	DefaultHeartBeatInterval = 5 // seconds

	ProgrammingLanguagePython = "python"
	ProgrammingLanguageBash   = "bash"

	RedisAvailableWorkersKey   = "codemode:workers:available"
	RedisWorkerCapacitiesKey   = "codemode:workers:capacity"
	RedisWorkerCapacityPrefix  = "codemode:worker:capacity:"
	RedisWorkerHeartbeatPrefix = "codemode:worker:heartbeat:"
)
