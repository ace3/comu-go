package models

import (
	"time"

	"gorm.io/datatypes"
)

// Schedule represents a train departure schedule at a specific station.
type Schedule struct {
	ID            string         `gorm:"primaryKey" json:"id"`
	TrainID       string         `gorm:"index;index:idx_schedules_train_departs,priority:1" json:"train_id"`
	Line          string         `json:"line"`
	Route         string         `json:"route"`
	OriginID      string         `json:"origin_id"`
	DestinationID string         `json:"destination_id"`
	StationID     string         `gorm:"index" json:"station_id"`
	DepartsAt     time.Time      `gorm:"index:idx_schedules_train_departs,priority:2" json:"departs_at"`
	ArrivesAt     time.Time      `json:"arrives_at"`
	Metadata      datatypes.JSON `gorm:"type:jsonb" json:"metadata" swaggertype:"object"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
