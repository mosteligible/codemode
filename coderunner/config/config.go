package config

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

type Config struct {
	RemoteHosts []string
	AppUserName string

	RedisHost     string
	RedisPort     string
	RedisUser     string
	RedisPassword string
	RedisDB       int

	DatabaseHost     string
	DatabaseName     string
	DatabasePort     string
	DatabaseUser     string
	DatabasePassword string

	lock sync.Mutex
}

func (c *Config) ReloadConfig() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c = NewConfig()
}

func NewConfig() *Config {
	godotenv.Load()
	remoteHosts := parseRemoteHosts(os.Getenv("REMOTE_HOSTS"))
	redisDb, err := strconv.Atoi(os.Getenv("REDIS_DB"))
	if err != nil {
		redisDb = 0
	}
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6379"
	}

	return &Config{
		RemoteHosts:   remoteHosts,
		AppUserName:   os.Getenv("APP_USER_NAME"),
		RedisHost:     redisHost,
		RedisPort:     redisPort,
		RedisUser:     os.Getenv("REDIS_USER"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       redisDb,

		DatabaseHost:     os.Getenv("DATABASE_HOST"),
		DatabaseName:     os.Getenv("DATABASE_NAME"),
		DatabasePort:     os.Getenv("DATABASE_PORT"),
		DatabaseUser:     os.Getenv("DATABASE_USER"),
		DatabasePassword: os.Getenv("DATABASE_PASSWORD"),

		lock: sync.Mutex{},
	}
}

func parseRemoteHosts(raw string) []string {
	parts := strings.Split(raw, ";")
	hosts := make([]string, 0, len(parts))
	for _, part := range parts {
		host := strings.TrimSpace(part)
		if host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}
