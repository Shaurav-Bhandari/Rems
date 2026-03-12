package config

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// ============================================================================
// REDIS CLIENT FACTORY
// Centralized Redis client creation with health checks and retry logic.
// ============================================================================

// RedisConfig holds Redis connection parameters.
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// NewRedisClient creates a new Redis client with the given config.
// Validates the connection by issuing a PING before returning.
func NewRedisClient(cfg RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
		MinIdleConns: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: failed to connect to %s:%s: %w", cfg.Host, cfg.Port, err)
	}

	log.Printf("✓ Redis connected at %s:%s", cfg.Host, cfg.Port)
	return client, nil
}

// LoadRedisConfig loads Redis config from environment variables.
func LoadRedisConfig() RedisConfig {
	return RedisConfig{
		Host:     GetEnvars("REDIS_HOST", "localhost"),
		Port:     GetEnvars("REDIS_PORT", "6379"),
		Password: GetEnvars("REDIS_PASSWORD", ""),
		DB:       0,
	}
}
