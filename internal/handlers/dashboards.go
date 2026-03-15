package handlers

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"volt/internal/database"
	"volt/internal/models"

	"github.com/gofiber/fiber/v2"
)

// GetDashboardPanelHandler renders the dashboard switcher modal.
func GetDashboardPanelHandler(c *fiber.Ctx) error {
	var dashboards []models.Dashboard
	if err := database.DB.Order("created_at asc, id asc").Find(&dashboards).Error; err != nil {
		log.Printf("dashboard-panel: failed to fetch dashboards: %v", err)
	}

	currentID, _ := getCurrentDashboardID(c)

	return c.Render("partials/dashboard_panel", fiber.Map{
		"Dashboards": dashboards,
		"CurrentID":  currentID,
	})
}

// CreateDashboardHandler creates a new dashboard and switches to it.
func CreateDashboardHandler(c *fiber.Ctx) error {
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}
	if len(name) > 50 {
		return c.Status(400).SendString("Name must be 50 characters or fewer")
	}

	dash := models.Dashboard{Name: name}
	if err := database.DB.Create(&dash).Error; err != nil {
		return c.Status(500).SendString("Error creating dashboard")
	}

	database.SeedDefaultSettings(dash.ID, name)

	sess, err := Store.Get(c)
	if err != nil {
		return c.Status(500).SendString("Session error")
	}
	sess.Set("dashboard_id", dash.ID)
	if err := sess.Save(); err != nil {
		log.Printf("CreateDashboardHandler: failed to save session: %v", err)
	}

	c.Set("HX-Redirect", "/")
	return c.SendStatus(200)
}

// SwitchDashboardHandler sets the active dashboard in the session and redirects home.
func SwitchDashboardHandler(c *fiber.Ctx) error {
	raw := c.Params("id")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	var dash models.Dashboard
	if err := database.DB.First(&dash, id).Error; err != nil {
		return c.Status(404).SendString("Dashboard not found")
	}

	sess, err := Store.Get(c)
	if err != nil {
		return c.Status(500).SendString("Session error")
	}
	sess.Set("dashboard_id", dash.ID)
	if err := sess.Save(); err != nil {
		log.Printf("SwitchDashboardHandler: failed to save session: %v", err)
	}

	c.Set("HX-Redirect", "/")
	return c.SendStatus(200)
}

// RenameDashboardHandler updates the name of an existing dashboard.
func RenameDashboardHandler(c *fiber.Ctx) error {
	raw := c.Params("id")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}

	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return c.Status(400).SendString("Name is required")
	}
	if len(name) > 50 {
		return c.Status(400).SendString("Name must be 50 characters or fewer")
	}

	var dash models.Dashboard
	if err := database.DB.First(&dash, id).Error; err != nil {
		return c.Status(404).SendString("Dashboard not found")
	}

	if err := database.DB.Model(&dash).Update("name", name).Error; err != nil {
		return c.Status(500).SendString("Error renaming dashboard")
	}

	// Re-render the dashboard panel so the UI reflects the new name.
	var dashboards []models.Dashboard
	if err := database.DB.Order("created_at asc, id asc").Find(&dashboards).Error; err != nil {
		log.Printf("rename-dashboard: failed to fetch dashboards: %v", err)
	}
	currentID, _ := getCurrentDashboardID(c)

	return c.Render("partials/dashboard_panel", fiber.Map{
		"Dashboards": dashboards,
		"CurrentID":  currentID,
	})
}

// DeleteDashboardHandler deletes a dashboard and all its links and settings.
// It refuses to delete the last remaining dashboard.
func DeleteDashboardHandler(c *fiber.Ctx) error {
	raw := c.Params("id")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid ID")
	}
	dashID := uint(id)

	// Refuse to delete the last dashboard.
	var count int64
	database.DB.Model(&models.Dashboard{}).Count(&count)
	if count <= 1 {
		return c.Status(400).SendString("Cannot delete the last dashboard")
	}

	// Verify it exists.
	var dash models.Dashboard
	if err := database.DB.First(&dash, dashID).Error; err != nil {
		return c.Status(404).SendString("Dashboard not found")
	}

	// Remove uploaded icons for every link on this dashboard.
	var links []models.Link
	if err := database.DB.Where("dashboard_id = ?", dashID).Find(&links).Error; err != nil {
		log.Printf("delete-dashboard: failed to fetch links for icon cleanup: %v", err)
	}
	for _, link := range links {
		if strings.HasPrefix(link.Icon, "/uploads/") {
			if err := os.Remove(filepath.Join("./uploads", filepath.Base(link.Icon))); err != nil && !os.IsNotExist(err) {
				log.Printf("DeleteDashboardHandler: failed to remove icon for link %d: %v", link.ID, err)
			}
		}
	}

	if err := database.DB.Where("dashboard_id = ?", dashID).Delete(&models.Link{}).Error; err != nil {
		log.Printf("delete-dashboard: failed to delete links: %v", err)
	}
	if err := database.DB.Where("dashboard_id = ?", dashID).Delete(&models.Setting{}).Error; err != nil {
		log.Printf("delete-dashboard: failed to delete settings: %v", err)
	}
	if err := database.DB.Delete(&models.Dashboard{}, dashID).Error; err != nil {
		return c.Status(500).SendString("Error deleting dashboard")
	}

	// If the deleted dashboard was active, switch to the first remaining one.
	sess, err := Store.Get(c)
	if err == nil {
		if currentID, ok := sess.Get("dashboard_id").(uint); ok && currentID == dashID {
			var first models.Dashboard
			database.DB.Order("created_at asc, id asc").First(&first)
			sess.Set("dashboard_id", first.ID)
			if saveErr := sess.Save(); saveErr != nil {
				log.Printf("DeleteDashboardHandler: failed to save session: %v", saveErr)
			}
		}
	}

	c.Set("HX-Redirect", "/")
	return c.SendStatus(200)
}
