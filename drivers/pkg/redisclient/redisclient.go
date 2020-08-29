package redisclient

import (
	"os"
	"strconv"

	"github.com/go-redis/redis/v8"
	"github.com/honeydipper/honeydipper/pkg/dipper"
)

// GetRedisOps configures driver to talk to Redis.
func GetRedisOps(driver *dipper.Driver) *redis.Options {
	if localRedis, ok := os.LookupEnv("LOCALREDIS"); ok && localRedis != "" {
		return &redis.Options{
			Addr: "127.0.0.1:6379",
			DB:   0,
		}
	}
	opts := &redis.Options{}
	if value, ok := driver.GetOptionStr("data.connection.Addr"); ok {
		opts.Addr = value
	}
	if value, ok := driver.GetOptionStr("data.connection.Password"); ok {
		opts.Password = value
	}
	if DB, ok := driver.GetOptionStr("data.connection.DB"); ok {
		opts.DB = dipper.Must(strconv.Atoi(DB)).(int)
	}
	return opts
}
