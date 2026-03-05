package models

import (
	"time"

	"gorm.io/datatypes"
)

// BotUser represents a Telegram bot user with their commute preferences.
type BotUser struct {
	TelegramID    int64          `gorm:"primaryKey" json:"telegram_id"`
	HomeStation   datatypes.JSON `gorm:"type:jsonb" json:"home_station" swaggertype:"object"`
	AwayStation   datatypes.JSON `gorm:"type:jsonb" json:"away_station" swaggertype:"object"`
	MorningTime   string         `json:"morning_time"` // "07:00"
	EveningTime   string         `json:"evening_time"` // "17:30"
	WorkDays      datatypes.JSON `gorm:"type:jsonb" json:"work_days" swaggertype:"array,string"` // ["mon","tue","wed","thu","fri"]
	Notifications bool           `json:"notifications"`
	Lang          string         `gorm:"default:'en'" json:"lang"` // "en"|"id"
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// OneTimeAlert stores a scheduled one-time commute alert.
type OneTimeAlert struct {
	ID           string         `gorm:"primaryKey" json:"id"` // UUID
	TelegramID   int64          `gorm:"index" json:"telegram_id"`
	Origin       datatypes.JSON `gorm:"type:jsonb" json:"origin" swaggertype:"object"`
	Dest         datatypes.JSON `gorm:"type:jsonb" json:"dest" swaggertype:"object"`
	ScheduledFor time.Time      `json:"scheduled_for"`
	SentAt       *time.Time     `json:"sent_at,omitempty"`
	Success      *bool          `json:"success,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}
