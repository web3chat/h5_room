package main

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisTimeout bounds every individual Redis operation (dial + command),
// so a dead Redis never hangs a request for long.
const redisTimeout = 2 * time.Second

var (
	// rdb is the shared Redis client. redis.NewClient is lazy (never dials),
	// so rdb stays non-nil even when Redis is down — room functions surface
	// connection errors instead of panicking.
	rdb *redis.Client
	// roomPinTTL is the room PIN expiry (0 = never); set by initRedis from config.
	roomPinTTL = time.Duration(0)
)

// initRedis creates the shared client and verifies connectivity.
// Warning (not fatal) on failure, matching the codebase's startup style.
func initRedis(cfg RedisConfig) {
	rdb = redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("warning: Redis 连接失败 (%v) — 房间 PIN 无法持久化, 相关接口将返回错误", err)
		return
	}
	roomPinTTL = time.Duration(cfg.PinTTLSeconds) * time.Second
	if roomPinTTL > 0 {
		log.Printf("  房间 PIN TTL: %s", roomPinTTL)
	}
}

// redisCtx returns a short-lived context for one Redis operation.
func redisCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), redisTimeout)
}
