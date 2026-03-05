// Package bot provides alert scheduling logic backed by Redis sorted sets.
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/comuline/api/internal/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const alertsKey = "krl:alerts"

// AlertPayload is the value stored in the Redis ZSET.
type AlertPayload struct {
	ID         string         `json:"id"`
	TelegramID int64          `json:"telegram_id"`
	OriginName string         `json:"origin_name"`
	OriginCode string         `json:"origin_code"`
	OriginLat  float64        `json:"origin_lat"`
	OriginLon  float64        `json:"origin_lon"`
	DestName   string         `json:"dest_name"`
	DestCode   string         `json:"dest_code"`
	SendTime   time.Time      `json:"send_time"`
}

// ScheduleAlert adds a one-time alert to the Redis ZSET and Postgres.
func ScheduleAlert(ctx context.Context, rc *redis.Client, db *gorm.DB, alert *models.OneTimeAlert, payload AlertPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("alert: marshal payload: %w", err)
	}

	score := float64(payload.SendTime.Unix())
	if err := rc.ZAdd(ctx, alertsKey, redis.Z{Score: score, Member: string(data)}).Err(); err != nil {
		return fmt.Errorf("alert: zadd: %w", err)
	}

	if err := db.WithContext(ctx).Create(alert).Error; err != nil {
		return fmt.Errorf("alert: db create: %w", err)
	}

	return nil
}

// PendingAlerts returns all alerts whose send time is <= now.
func PendingAlerts(ctx context.Context, rc *redis.Client) ([]AlertPayload, error) {
	now := float64(time.Now().Unix())
	results, err := rc.ZRangeByScore(ctx, alertsKey, &redis.ZRangeBy{
		Min: "-inf",
		Max: fmt.Sprintf("%f", now),
	}).Result()
	if err != nil {
		return nil, err
	}

	var payloads []AlertPayload
	for _, r := range results {
		var p AlertPayload
		if err := json.Unmarshal([]byte(r), &p); err != nil {
			continue
		}
		payloads = append(payloads, p)
	}
	return payloads, nil
}

// RemoveAlerts removes processed alert entries from the ZSET by their JSON member values.
func RemoveAlerts(ctx context.Context, rc *redis.Client, members []string) error {
	if len(members) == 0 {
		return nil
	}
	args := make([]interface{}, len(members))
	for i, m := range members {
		args[i] = m
	}
	return rc.ZRem(ctx, alertsKey, args...).Err()
}

// ListUserAlerts returns pending alerts from Postgres for a user (not yet sent).
func ListUserAlerts(ctx context.Context, db *gorm.DB, telegramID int64) ([]models.OneTimeAlert, error) {
	var alerts []models.OneTimeAlert
	err := db.WithContext(ctx).
		Where("telegram_id = ? AND sent_at IS NULL AND scheduled_for > ?", telegramID, time.Now()).
		Order("scheduled_for asc").
		Find(&alerts).Error
	return alerts, err
}

// CancelAlert deletes an alert from Postgres and removes it from Redis by matching the prefix of its ID.
func CancelAlert(ctx context.Context, rc *redis.Client, db *gorm.DB, telegramID int64, idPrefix string) error {
	var alert models.OneTimeAlert
	if err := db.WithContext(ctx).
		Where("telegram_id = ? AND id LIKE ? AND sent_at IS NULL", telegramID, idPrefix+"%").
		First(&alert).Error; err != nil {
		return err
	}

	if err := db.WithContext(ctx).Delete(&alert).Error; err != nil {
		return err
	}

	// Remove from Redis ZSET by scanning for members containing the full ID.
	members, err := rc.ZRange(ctx, alertsKey, 0, -1).Result()
	if err != nil {
		return nil // non-fatal
	}
	var toRemove []string
	for _, m := range members {
		if strings.Contains(m, alert.ID) {
			toRemove = append(toRemove, m)
		}
	}
	if len(toRemove) > 0 {
		_ = RemoveAlerts(ctx, rc, toRemove)
	}

	return nil
}
