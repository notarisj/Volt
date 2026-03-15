package database

import (
	"fmt"
	"log"
	"os"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"volt/internal/models"
)

var DB *gorm.DB

func Init() {
	var err error

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "volt.db"
	}

	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	runMigrations()
	ensureDefaultDashboard()
}

func runMigrations() {
	// Always ensure the dashboards table exists first
	if err := DB.AutoMigrate(&models.Dashboard{}); err != nil {
		log.Fatal("Failed to migrate dashboards table:", err)
	}

	// If the links table already exists but lacks dashboard_id, perform the
	// one-time migration from the single-dashboard schema to multi-dashboard.
	if tableExists("links") && !columnExists("links", "dashboard_id") {
		migrateToMultiDashboard()
	}

	// AutoMigrate the rest — on a fresh DB this creates all tables with the
	// correct schema; on an existing DB it only adds any missing columns.
	if err := DB.AutoMigrate(&models.Link{}, &models.Setting{}, &models.User{}); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}
}

// migrateToMultiDashboard performs a one-time schema upgrade:
//   - Creates a "Home" dashboard and assigns all existing links to it.
//   - Recreates the settings table with a composite (dashboard_id, key) primary key,
//     copying all existing settings rows to the new default dashboard.
func migrateToMultiDashboard() {
	log.Println("database: migrating to multi-dashboard schema…")

	// Create the default dashboard that will own all existing data.
	defaultDash := models.Dashboard{Name: "Home"}
	if err := DB.Create(&defaultDash).Error; err != nil {
		log.Fatal("database: failed to create default dashboard during migration:", err)
	}

	// Add dashboard_id column to links, defaulting all rows to the new dashboard.
	DB.Exec(fmt.Sprintf(
		"ALTER TABLE links ADD COLUMN dashboard_id INTEGER NOT NULL DEFAULT %d",
		defaultDash.ID,
	))

	// Recreate the settings table with the new composite primary key.
	DB.Exec(`CREATE TABLE IF NOT EXISTS settings_new (
		dashboard_id INTEGER NOT NULL,
		key          TEXT    NOT NULL,
		value        TEXT    NOT NULL,
		updated_at   DATETIME,
		PRIMARY KEY (dashboard_id, key)
	)`)

	if tableExists("settings") {
		DB.Exec(
			fmt.Sprintf("INSERT INTO settings_new SELECT %d, key, value, updated_at FROM settings", defaultDash.ID),
		)
		DB.Exec("DROP TABLE settings")
	}

	DB.Exec("ALTER TABLE settings_new RENAME TO settings")

	log.Printf("database: migration complete — all existing data assigned to dashboard %d (%s)", defaultDash.ID, defaultDash.Name)
}

// ensureDefaultDashboard creates the very first dashboard (fresh installs only)
// and seeds its default settings.
func ensureDefaultDashboard() {
	var count int64
	DB.Model(&models.Dashboard{}).Count(&count)
	if count > 0 {
		return
	}

	defaultDash := models.Dashboard{Name: "Home"}
	if err := DB.Create(&defaultDash).Error; err != nil {
		log.Fatal("database: failed to create default dashboard:", err)
	}
	SeedDefaultSettings(defaultDash.ID, "Your Dashboard")
}

// SeedDefaultSettings inserts missing default settings for a dashboard.
// title is used as the dashboard_title value; pass "" to use the generic default.
func SeedDefaultSettings(dashboardID uint, title string) {
	if title == "" {
		title = "Your Dashboard"
	}

	defaults := []models.Setting{
		{DashboardID: dashboardID, Key: "dashboard_title", Value: title},
		{DashboardID: dashboardID, Key: "dashboard_subtitle", Value: "Lightning fast access to your favorite applications."},
		{DashboardID: dashboardID, Key: "is_compact_mode", Value: "true"},
		{DashboardID: dashboardID, Key: "animations_enabled", Value: "true"},
		{DashboardID: dashboardID, Key: "grid_columns", Value: "6"},
		{DashboardID: dashboardID, Key: "theme_mode", Value: "dark"},
	}

	for _, s := range defaults {
		var existing int64
		DB.Model(&models.Setting{}).
			Where("dashboard_id = ? AND key = ?", s.DashboardID, s.Key).
			Count(&existing)
		if existing == 0 {
			DB.Create(&s)
		}
	}
}

// tableExists reports whether the named table is present in the SQLite database.
func tableExists(name string) bool {
	var count int64
	DB.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count)
	return count > 0
}

// columnExists reports whether a column exists in the given table.
func columnExists(table, column string) bool {
	var count int64
	DB.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?", table, column).Scan(&count)
	return count > 0
}
