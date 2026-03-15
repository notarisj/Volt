package models

import (
	"time"
)

// Link represents a visual link card on the dashboard
type Link struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	DashboardID uint      `gorm:"not null;index" json:"dashboard_id"`
	Name        string    `gorm:"not null" json:"name"`
	URL         string    `gorm:"not null" json:"url"`
	Icon        string    `json:"icon"`
	OrderIndex  int       `json:"order_index"`
	// Grid layout fields
	GridCol   int       `gorm:"default:1" json:"grid_col"`
	GridRow   int       `gorm:"default:1" json:"grid_row"`
	ColSpan   int       `gorm:"default:1" json:"col_span"`
	RowSpan   int       `gorm:"default:1" json:"row_span"`
	CreatedAt time.Time `json:"created_at"`
}
