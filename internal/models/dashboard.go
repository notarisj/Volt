package models

import "time"

// Dashboard represents an isolated set of links and settings
type Dashboard struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
