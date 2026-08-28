package middleware

// Building the process-wide credential limiter.
//
// The router constructs the guard on first use rather than taking it as a
// parameter, because the router's constructor is called positionally from the
// API entry point and every added parameter breaks that call. SetCredentialLimiter
// exists so the entry point can inject the connection it already holds once it
// is convenient to do so; until then this resolves one from configuration.
//
// Note what happens when Redis is unreachable: nothing here fails. The client
// is created without a round trip, and each request discovers the outage
// through the guard, which fails closed and logs why. A panel that cannot
// count attempts refuses them.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/ratelimit"
)

func buildCredentialLimiter(logger *zap.Logger) *ratelimit.Guard {
	addr, password, db := redisTarget(logger)

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	policy := ratelimit.PolicyFromEnv()
	if logger != nil {
		logger.Info("credential limiter ready",
			zap.String("redis", addr),
			zap.Int("pair_lock_threshold", policy.PairLockThreshold),
			zap.Int("address_limit", policy.AddressLimit),
			zap.Int("account_limit", policy.AccountLimit),
			zap.Bool("fail_open", policy.FailOpen),
			zap.String("auth_log", AuthLogPath()))
		if policy.FailOpen {
			logger.Warn("credential limiter is configured to fail open: " +
				"an attacker who can take Redis down can switch brute force protection off")
		}
	}

	return ratelimit.New(ratelimit.NewRedisStore(client), policy)
}

// redisTarget resolves where Redis is. The loaded configuration is preferred
// because it is what the rest of the process uses; the environment is the
// fallback for a process that has not loaded configuration yet.
func redisTarget(logger *zap.Logger) (addr, password string, db int) {
	if cfg, err := config.Load(); err == nil && cfg != nil {
		return fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port), cfg.Redis.Password, cfg.Redis.DB
	} else if err != nil && logger != nil {
		logger.Warn("credential limiter falling back to environment for Redis settings",
			zap.Error(err))
	}

	host := firstNonEmpty(os.Getenv("VKAI_REDIS_HOST"), os.Getenv("REDIS_HOST"), "localhost")
	port := 6379
	if p, err := strconv.Atoi(strings.TrimSpace(firstNonEmpty(os.Getenv("VKAI_REDIS_PORT"), os.Getenv("REDIS_PORT")))); err == nil && p > 0 {
		port = p
	}
	password = firstNonEmpty(os.Getenv("VKAI_REDIS_PASSWORD"), os.Getenv("REDIS_PASSWORD"))
	if n, err := strconv.Atoi(strings.TrimSpace(firstNonEmpty(os.Getenv("VKAI_REDIS_DB"), os.Getenv("REDIS_DB")))); err == nil && n >= 0 {
		db = n
	}

	return fmt.Sprintf("%s:%d", host, port), password, db
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
