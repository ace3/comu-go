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

// MenuState represents the current button-driven navigation screen for a user.
type MenuState struct {
	Screen  string            `json:"screen"`
	Version int               `json:"version"`
	Data    map[string]string `json:"data"`
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

func menuKey(userID int64) string {
	return fmt.Sprintf("krl:menu:%d", userID)
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

// GetMenu retrieves the current menu state for the user. Returns nil if absent.
func (s *Store) GetMenu(ctx context.Context, userID int64) (*MenuState, error) {
	val, err := s.client.Get(ctx, menuKey(userID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st MenuState
	if err := json.Unmarshal([]byte(val), &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// SetMenu stores the current menu state for the user.
func (s *Store) SetMenu(ctx context.Context, userID int64, st *MenuState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, menuKey(userID), data, s.ttl).Err()
}

// ClearMenu deletes the current menu state for the user.
func (s *Store) ClearMenu(ctx context.Context, userID int64) error {
	return s.client.Del(ctx, menuKey(userID)).Err()
}
