package redisclient

import (
	"os"
	"strconv"

	"github.com/go-redis/redis"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"github.com/op/go-logging"
)

var log *logging.Logger

// GetRedisOps configures driver to talk to Redis
func GetRedisOps(driver *dipper.Driver) *redis.Options {
	opts := &redis.Options{}
	if localRedis, ok := os.LookupEnv("LOCALREDIS"); ok && localRedis != "" {
		opts.Addr = "127.0.0.1:6379"
		opts.DB = 0
	} else {
		if value, ok := driver.GetOptionStr("data.connection.Addr"); ok {
			opts.Addr = value
		}
		if value, ok := driver.GetOptionStr("data.connection.Password"); ok {
			opts.Password = value
		}
		if DB, ok := driver.GetOptionStr("data.connection.DB"); ok {
			DBnum, err := strconv.Atoi(DB)
			if err != nil {
				log.Panicf("[%s] invalid db number %s", driver.Service, DB)
			}
			opts.DB = DBnum
		}
	}
	return opts
}
