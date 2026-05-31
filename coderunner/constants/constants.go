package constants

const (
	GITHUB_BASE_URL          = "https://api.github.com"
	GITHUB_BASE              = "github"
	MICROSOFT_GRAPH_BASE_URL = "https://graph.microsoft.com/v1.0"
	MICROSOFT_GRAPH_BASE     = "graph"
	PROXY_HEADER_KEY         = "X-Proxy-Id"

	RedisAvailableWorkersKey  = "codemode:workers:available"
	RedisWorkerCapacitiesKey  = "codemode:workers:capacity"
	RedisWorkerCapacityPrefix = "codemode:worker:capacity:"
	RedisSessionWorkerPrefix  = "codemode:session:worker:"
	RedisWorkerSessionsPrefix = "codemode:worker:sessions:"
)
