package models

import (
	"time"

	"gorm.io/datatypes"
)

// Station represents a KRL commuter line station.
type Station struct {
	UID       string         `gorm:"primaryKey" json:"uid"`
	ID        string         `gorm:"uniqueIndex" json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"` // KRL, MRT, LRT, LOCAL
	Metadata  datatypes.JSON `gorm:"type:jsonb" json:"metadata" swaggertype:"object"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}
