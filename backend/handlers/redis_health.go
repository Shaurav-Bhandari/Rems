package handlers

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

// ============================================================================
// REDIS HEALTH SERVICE
// Verifies Redis connectivity and session integrity.
// ============================================================================

type RedisHealthService struct {
	redis *redis.Client
}

func NewRedisHealthService(redisClient *redis.Client) *RedisHealthService {
	return &RedisHealthService{redis: redisClient}
}

// Ping verifies Redis connectivity.
func (s *RedisHealthService) Ping(ctx context.Context) error {
	return s.redis.Ping(ctx).Err()
}

// GetStats returns Redis server statistics.
func (s *RedisHealthService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	dbSize, err := s.redis.DBSize(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("redis DBSize failed: %w", err)
	}

	info, err := s.redis.Info(ctx, "memory").Result()
	if err != nil {
		return nil, fmt.Errorf("redis Info failed: %w", err)
	}

	return map[string]interface{}{
		"total_keys":  dbSize,
		"memory_info": info,
	}, nil
}

// VerifySessionIntegrity uses SSCAN to check session set integrity for a user.
// Removes orphaned session IDs from the user's set.
func (s *RedisHealthService) VerifySessionIntegrity(ctx context.Context, userID string) (int, int, error) {
	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID)

	var cursor uint64
	var orphaned int
	var valid int

	for {
		keys, nextCursor, err := s.redis.SScan(ctx, userSessionsKey, cursor, "*", 100).Result()
		if err != nil {
			return 0, 0, fmt.Errorf("SSCAN failed: %w", err)
		}

		for _, sessionID := range keys {
			sessionKey := fmt.Sprintf("session:%s:%s", userID, sessionID)
			exists, err := s.redis.Exists(ctx, sessionKey).Result()
			if err != nil {
				continue
			}

			if exists == 0 {
				// Orphaned session ID — remove from set
				s.redis.SRem(ctx, userSessionsKey, sessionID)
				orphaned++
				log.Printf("[REDIS-HEALTH] Removed orphaned session %s for user %s", sessionID, userID)
			} else {
				valid++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return valid, orphaned, nil
}
