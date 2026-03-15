package models

import "time"

// Setting represents a key-value configuration scoped to a dashboard
type Setting struct {
	DashboardID uint      `gorm:"primaryKey" json:"dashboard_id"`
	Key         string    `gorm:"primaryKey" json:"key"`
	Value       string    `gorm:"not null" json:"value"`
	UpdatedAt   time.Time `json:"updated_at"`
}
