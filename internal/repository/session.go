package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mmryalloc/tody/internal/domain"
	"github.com/redis/go-redis/v9"
)

const sessionScanCount = 100

type sessionRecord struct {
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"ip_address"`
	CreatedAt time.Time `json:"created_at"`
}

func sessionRecordFromEntity(s domain.Session) sessionRecord {
	return sessionRecord{
		UserAgent: s.UserAgent,
		IPAddress: s.IPAddress,
		CreatedAt: s.CreatedAt,
	}
}

type sessionRepository struct {
	client *redis.Client
}

func NewSessionRepository(client *redis.Client) *sessionRepository {
	return &sessionRepository{client: client}
}

func sessionKey(userID int64, tokenHash string) string {
	return "session:" + strconv.FormatInt(userID, 10) + ":" + tokenHash
}

func lookupKey(tokenHash string) string {
	return "refresh_lookup:" + tokenHash
}

func userSessionsPattern(userID int64) string {
	return "session:" + strconv.FormatInt(userID, 10) + ":*"
}

func (r *sessionRepository) Save(ctx context.Context, userID int64, tokenHash string, s domain.Session, ttl time.Duration) error {
	payload, err := json.Marshal(sessionRecordFromEntity(s))
	if err != nil {
		return fmt.Errorf("repository session marshal: %w", err)
	}

	pipe := r.client.TxPipeline()
	pipe.Set(ctx, sessionKey(userID, tokenHash), payload, ttl)
	pipe.Set(ctx, lookupKey(tokenHash), strconv.FormatInt(userID, 10), ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("repository session save: %w", err)
	}
	return nil
}

func (r *sessionRepository) Exists(ctx context.Context, userID int64, tokenHash string) (bool, error) {
	n, err := r.client.Exists(ctx, sessionKey(userID, tokenHash)).Result()
	if err != nil {
		return false, fmt.Errorf("repository session exists: %w", err)
	}
	return n > 0, nil
}

func (r *sessionRepository) LookupUserID(ctx context.Context, tokenHash string) (int64, error) {
	val, err := r.client.Get(ctx, lookupKey(tokenHash)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, domain.ErrSessionNotFound
		}
		return 0, fmt.Errorf("repository session lookup: %w", err)
	}
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("repository session lookup parse: %w", err)
	}
	return id, nil
}

func (r *sessionRepository) Delete(ctx context.Context, userID int64, tokenHash string) error {
	if err := r.client.Del(ctx, sessionKey(userID, tokenHash), lookupKey(tokenHash)).Err(); err != nil {
		return fmt.Errorf("repository session delete: %w", err)
	}
	return nil
}

func (r *sessionRepository) DeleteAllForUser(ctx context.Context, userID int64) error {
	return r.deleteAllForUser(ctx, userID, "")
}

func (r *sessionRepository) DeleteAllForUserExcept(ctx context.Context, userID int64, keepTokenHash string) error {
	return r.deleteAllForUser(ctx, userID, keepTokenHash)
}

func (r *sessionRepository) deleteAllForUser(ctx context.Context, userID int64, keepTokenHash string) error {
	pattern := userSessionsPattern(userID)
	iter := r.client.Scan(ctx, 0, pattern, sessionScanCount).Iterator()

	prefix := "session:" + strconv.FormatInt(userID, 10) + ":"
	batch := make([]string, 0, sessionScanCount*2)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := r.client.Del(ctx, batch...).Err(); err != nil {
			return fmt.Errorf("repository session delete all: %w", err)
		}
		batch = batch[:0]
		return nil
	}

	for iter.Next(ctx) {
		key := iter.Val()
		hash, ok := strings.CutPrefix(key, prefix)
		if ok && hash == keepTokenHash {
			continue
		}
		batch = append(batch, key)
		if ok {
			batch = append(batch, lookupKey(hash))
		}
		if len(batch) >= sessionScanCount*2 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("repository session scan: %w", err)
	}

	return flush()
}
