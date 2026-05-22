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
	remoteHosts := os.Getenv("REMOTE_HOSTS")
	redisDb, err := strconv.Atoi(os.Getenv("REDIS_DB"))
	if err != nil {
		redisDb = 0
	}

	return &Config{
		RemoteHosts:   strings.Split(remoteHosts, ";"),
		AppUserName:   os.Getenv("APP_USER_NAME"),
		RedisHost:     os.Getenv("REDIS_HOST"),
		RedisPort:     os.Getenv("REDIS_PORT"),
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
