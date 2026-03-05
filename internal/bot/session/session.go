// Package session manages per-user conversation state stored in Redis.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// State represents a user's current conversation state.
type State struct {
	Command string            `json:"command"` // active command, e.g. "set_route"
	Step    int               `json:"step"`    // step within the command flow
	Data    map[string]string `json:"data"`    // temporary key-value pairs
}

// Store manages session state in Redis.
type Store struct {
	client *redis.Client
	ttl    time.Duration
}

// New creates a Store backed by the given Redis client.
func New(client *redis.Client) *Store {
	return &Store{
		client: client,
		ttl:    30 * time.Minute,
	}
}

func key(userID int64) string {
	return fmt.Sprintf("krl:sessions:%d", userID)
}

// Get retrieves the session state for the user. Returns nil if no state exists.
func (s *Store) Get(ctx context.Context, userID int64) (*State, error) {
	val, err := s.client.Get(ctx, key(userID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal([]byte(val), &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// Set stores the session state for the user.
func (s *Store) Set(ctx context.Context, userID int64, st *State) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key(userID), data, s.ttl).Err()
}

// Clear deletes the session state for the user.
func (s *Store) Clear(ctx context.Context, userID int64) error {
	return s.client.Del(ctx, key(userID)).Err()
}
