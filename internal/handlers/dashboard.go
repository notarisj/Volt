package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"volt/internal/database"
	"volt/internal/models"
	"volt/internal/version"

	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v2"
)

// getCurrentDashboardID reads the active dashboard ID from the session and
// validates that the dashboard still exists.  Falls back to the first
// dashboard if the session carries a stale or missing value.
func getCurrentDashboardID(c *fiber.Ctx) (uint, error) {
	sess, err := Store.Get(c)
	if err != nil {
		return 0, fmt.Errorf("session error")
	}

	if raw := sess.Get("dashboard_id"); raw != nil {
		if id, ok := raw.(uint); ok && id > 0 {
			var count int64
			database.DB.Model(&models.Dashboard{}).Where("id = ?", id).Count(&count)
			if count > 0 {
				return id, nil
			}
		}
	}

	// Fall back to the oldest dashboard.
	var first models.Dashboard
	if err := database.DB.Order("created_at asc, id asc").First(&first).Error; err != nil {
		return 0, fmt.Errorf("no dashboards found")
	}

	sess.Set("dashboard_id", first.ID)
	if err := sess.Save(); err != nil {
		log.Printf("getCurrentDashboardID: failed to save session: %v", err)
	}
	return first.ID, nil
}

func validateLinkURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid URL: must be a full URL with a host")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("URL must start with http:// or https://")
	}
	return nil
}

var allowedImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true, ".ico": true,
	".svg": true, ".avif": true, ".bmp": true, ".tiff": true,
}

var rasterExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

func saveAndResizeIcon(c *fiber.Ctx, file *multipart.FileHeader) (string, error) {
	const maxSize = 5 * 1024 * 1024
	if file.Size > maxSize {
		return "", fmt.Errorf("file exceeds 5MB limit")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedImageExts[ext] {
		return "", fmt.Errorf("unsupported file type")
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate filename: %w", err)
	}
	filename := hex.EncodeToString(b) + ext
	savePath := filepath.Join("./uploads", filename)

	if err := c.SaveFile(file, savePath); err != nil {
		return "", err
	}

	if rasterExts[ext] {
		if img, err := imaging.Open(savePath); err == nil {
			dst := imaging.Fill(img, 200, 200, imaging.Center, imaging.Lanczos)
			if err := imaging.Save(dst, savePath); err != nil {
				log.Printf("warning: failed to resize icon %s: %v", filename, err)
			}
		}
	}

	return "/uploads/" + filename, nil
}

func getAutoIconPath(linkURL string) string {
	cleanURL := strings.TrimPrefix(strings.TrimPrefix(linkURL, "https://"), "http://")
	parts := strings.Split(cleanURL, "/")
	if len(parts) > 0 && parts[0] != "" {
		return fmt.Sprintf("https://www.google.com/s2/favicons?domain=%s&sz=128", parts[0])
	}
	return ""
}

// findFreeCell returns the first unoccupied (col, row) in a grid of width gridCols,
// scanning left-to-right, top-to-bottom, respecting the spans of all existing links.
func findFreeCell(gridCols int, existing []models.Link) (col, row int) {
	occupied := make(map[[2]int]bool)
	for _, l := range existing {
		cs, rs := l.ColSpan, l.RowSpan
		if cs < 1 {
			cs = 1
		}
		if rs < 1 {
			rs = 1
		}
		for r := 0; r < rs; r++ {
			for c := 0; c < cs; c++ {
				occupied[[2]int{l.GridCol + c, l.GridRow + r}] = true
			}
		}
	}
	for r := 1; r <= 100; r++ {
		for c := 1; c <= gridCols; c++ {
			if !occupied[[2]int{c, r}] {
				return c, r
			}
		}
	}
	return 1, 1
}

func IndexHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}

	var links []models.Link
	if err := database.DB.Where("dashboard_id = ?", dashID).
		Order("order_index asc, id asc").Find(&links).Error; err != nil {
		return c.Status(500).SendString("Error fetching links")
	}

	var settingsList []models.Setting
	if err := database.DB.Where("dashboard_id = ?", dashID).Find(&settingsList).Error; err != nil {
		log.Printf("index: failed to fetch settings: %v", err)
	}
	settings := make(map[string]string)
	for _, s := range settingsList {
		settings[s.Key] = s.Value
	}

	// theme_mode is global (not per-dashboard): read from session if set.
	if sess, sessErr := Store.Get(c); sessErr == nil {
		if t, ok := sess.Get("theme_mode").(string); ok && (t == "dark" || t == "light") {
			settings["theme_mode"] = t
		}
	}

	// Ensure grid_columns always has a valid value so the CSS --cols var is never empty.
	gridCols := 6
	if n, err := strconv.Atoi(settings["grid_columns"]); err == nil && n >= 1 {
		gridCols = n
	}
	settings["grid_columns"] = strconv.Itoa(gridCols)

	type cell struct{ c, r int }
	occupied := make(map[cell]bool)
	for _, l := range links {
		if l.GridCol > 0 && l.GridRow > 0 {
			cs := l.ColSpan
			if cs < 1 {
				cs = 1
			}
			rs := l.RowSpan
			if rs < 1 {
				rs = 1
			}
			for rr := 0; rr < rs; rr++ {
				for cc := 0; cc < cs; cc++ {
					occupied[cell{l.GridCol + cc, l.GridRow + rr}] = true
				}
			}
		}
	}
	for i := range links {
		if links[i].GridCol < 1 || links[i].GridRow < 1 {
			if links[i].ColSpan < 1 {
				links[i].ColSpan = 1
			}
			if links[i].RowSpan < 1 {
				links[i].RowSpan = 1
			}
			foundCol, foundRow := 1, 1
		outer:
			for row := 1; row <= 200; row++ {
				for col := 1; col <= gridCols; col++ {
					free := true
					for rr := 0; rr < links[i].RowSpan && free; rr++ {
						for cc := 0; cc < links[i].ColSpan && free; cc++ {
							if occupied[cell{col + cc, row + rr}] {
								free = false
							}
						}
					}
					if free {
						foundCol, foundRow = col, row
						break outer
					}
				}
			}
			links[i].GridCol = foundCol
			links[i].GridRow = foundRow
			for rr := 0; rr < links[i].RowSpan; rr++ {
				for cc := 0; cc < links[i].ColSpan; cc++ {
					occupied[cell{foundCol + cc, foundRow + rr}] = true
				}
			}
			if res := database.DB.Model(&models.Link{}).Where("id = ?", links[i].ID).Updates(map[string]interface{}{
				"grid_col": links[i].GridCol,
				"grid_row": links[i].GridRow,
				"col_span": links[i].ColSpan,
				"row_span": links[i].RowSpan,
			}); res.Error != nil {
				log.Printf("index: failed to persist auto-layout for link %d: %v", links[i].ID, res.Error)
			}
		}
	}

	var dashboards []models.Dashboard
	if err := database.DB.Order("created_at asc, id asc").Find(&dashboards).Error; err != nil {
		log.Printf("index: failed to fetch dashboards: %v", err)
	}

	var currentDash models.Dashboard
	if err := database.DB.First(&currentDash, dashID).Error; err != nil {
		log.Printf("index: failed to fetch current dashboard %d: %v", dashID, err)
	}

	return c.Render("index", fiber.Map{
		"Links":      links,
		"Settings":   settings,
		"Version":    version.Version,
		"Dashboards": dashboards,
		"Dashboard":  currentDash,
	}, "layouts/main")
}

func AddLinkHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}

	name := strings.TrimSpace(c.FormValue("name"))
	linkURL := strings.TrimSpace(c.FormValue("url"))

	if name == "" || linkURL == "" {
		return c.Status(400).SendString("Name and URL are required")
	}

	if len(name) > 255 {
		return c.Status(400).SendString("Name must be 255 characters or fewer")
	}

	if len(linkURL) > 2048 {
		return c.Status(400).SendString("URL must be 2048 characters or fewer")
	}

	if err := validateLinkURL(linkURL); err != nil {
		return c.Status(400).SendString(err.Error())
	}

	iconPath := ""
	file, err := c.FormFile("icon")
	if err == nil && file != nil {
		path, saveErr := saveAndResizeIcon(c, file)
		if saveErr != nil {
			return c.Status(400).SendString(saveErr.Error())
		}
		iconPath = path
	}

	if iconPath == "" {
		iconPath = getAutoIconPath(linkURL)
	}

	var count int64
	database.DB.Model(&models.Link{}).Where("dashboard_id = ?", dashID).Count(&count)

	gridCols := 6
	var colsSetting models.Setting
	if database.DB.Where("dashboard_id = ? AND key = ?", dashID, "grid_columns").First(&colsSetting).Error == nil {
		if n, err := strconv.Atoi(colsSetting.Value); err == nil && n >= 1 {
			gridCols = n
		}
	}
	var existing []models.Link
	if err := database.DB.Where("dashboard_id = ?", dashID).
		Select("grid_col, grid_row, col_span, row_span").Find(&existing).Error; err != nil {
		log.Printf("add-link: failed to fetch existing positions: %v", err)
	}
	freeCol, freeRow := findFreeCell(gridCols, existing)

	newLink := models.Link{
		DashboardID: dashID,
		Name:        name,
		URL:         linkURL,
		Icon:        iconPath,
		OrderIndex:  int(count),
		GridCol:     freeCol,
		GridRow:     freeRow,
		ColSpan:     1,
		RowSpan:     1,
	}

	if err := database.DB.Create(&newLink).Error; err != nil {
		return c.Status(500).SendString("Error creating link")
	}

	return c.Render("partials/link", newLink)
}

func GetDeleteLinkHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}
	id := c.Params("id")
	var link models.Link
	if err := database.DB.Where("id = ? AND dashboard_id = ?", id, dashID).First(&link).Error; err != nil {
		return c.Status(404).SendString("Link not found")
	}
	return c.Render("partials/delete_modal", link)
}

func RemoveLinkHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}
	id := c.Params("id")
	var link models.Link
	if err := database.DB.Where("id = ? AND dashboard_id = ?", id, dashID).First(&link).Error; err != nil {
		return c.Status(404).SendString("Link not found")
	}
	if err := database.DB.Delete(&models.Link{}, id).Error; err != nil {
		return c.Status(500).SendString("Error deleting link")
	}
	if strings.HasPrefix(link.Icon, "/uploads/") {
		if err := os.Remove(filepath.Join("./uploads", filepath.Base(link.Icon))); err != nil && !os.IsNotExist(err) {
			log.Printf("remove-link: failed to remove icon for link %d: %v", link.ID, err)
		}
	}
	return c.SendString("")
}

func GetEditLinkHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}
	id := c.Params("id")
	var link models.Link
	if err := database.DB.Where("id = ? AND dashboard_id = ?", id, dashID).First(&link).Error; err != nil {
		return c.Status(404).SendString("Link not found")
	}
	return c.Render("partials/edit_modal", link)
}

func EditLinkHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}
	id := c.Params("id")
	var link models.Link
	if err := database.DB.Where("id = ? AND dashboard_id = ?", id, dashID).First(&link).Error; err != nil {
		return c.Status(404).SendString("Link not found")
	}

	name := strings.TrimSpace(c.FormValue("name"))
	linkURL := strings.TrimSpace(c.FormValue("url"))
	if name == "" || linkURL == "" {
		return c.Status(400).SendString("Name and URL are required")
	}

	if len(name) > 255 {
		return c.Status(400).SendString("Name must be 255 characters or fewer")
	}

	if len(linkURL) > 2048 {
		return c.Status(400).SendString("URL must be 2048 characters or fewer")
	}

	if err := validateLinkURL(linkURL); err != nil {
		return c.Status(400).SendString(err.Error())
	}

	link.Name = name
	link.URL = linkURL

	file, err := c.FormFile("icon")
	if err == nil && file != nil {
		path, saveErr := saveAndResizeIcon(c, file)
		if saveErr != nil {
			return c.Status(400).SendString(saveErr.Error())
		}
		if strings.HasPrefix(link.Icon, "/uploads/") {
			os.Remove(filepath.Join("./uploads", filepath.Base(link.Icon)))
		}
		link.Icon = path
	} else if link.Icon == "" || strings.HasPrefix(link.Icon, "https://www.google.com/s2/favicons") {
		link.Icon = getAutoIconPath(linkURL)
	}

	if err := database.DB.Save(&link).Error; err != nil {
		return c.Status(500).SendString("Error updating link")
	}

	return c.Render("partials/link", link)
}

func SaveSettingsHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}

	boolSettings := []string{"is_compact_mode", "animations_enabled", "show_cpu_widget", "show_ram_widget"}
	submitted := make(map[string]bool)

	allowedSettingKeys := map[string]bool{
		"grid_columns": true, "grid_cell": true, "grid_gap": true,
		"theme_mode": true, "is_compact_mode": true, "animations_enabled": true,
		"dashboard_title": true, "dashboard_subtitle": true,
		"show_cpu_widget": true, "show_ram_widget": true,
	}

	validateSetting := func(key, value string) (string, bool) {
		if !allowedSettingKeys[key] {
			return "", false
		}
		switch key {
		case "grid_columns":
			n, err := strconv.Atoi(value)
			if err != nil || n < 2 || n > 10 {
				return "", false
			}
		case "grid_cell":
			n, err := strconv.Atoi(value)
			if err != nil || n < 80 || n > 200 {
				return "", false
			}
		case "grid_gap":
			n, err := strconv.Atoi(value)
			if err != nil || n < 4 || n > 48 {
				return "", false
			}
		case "theme_mode":
			if value != "dark" && value != "light" {
				return "", false
			}
		case "is_compact_mode", "animations_enabled", "show_cpu_widget", "show_ram_widget":
			if value != "true" && value != "false" {
				return "", false
			}
		case "dashboard_title", "dashboard_subtitle":
			if len(value) > 100 {
				return "", false
			}
		}
		return value, true
	}

	upsertSetting := func(key, value string) {
		var s models.Setting
		res := database.DB.Where("dashboard_id = ? AND key = ?", dashID, key).First(&s)
		if res.Error != nil {
			database.DB.Create(&models.Setting{
				DashboardID: dashID,
				Key:         key,
				Value:       value,
				UpdatedAt:   time.Now(),
			})
		} else {
			database.DB.Model(&s).Updates(map[string]interface{}{
				"value":      value,
				"updated_at": time.Now(),
			})
		}
		submitted[key] = true
	}

	form, err := c.MultipartForm()
	if err == nil {
		for key, values := range form.Value {
			if len(values) == 0 {
				continue
			}
			v, ok := validateSetting(key, values[0])
			if !ok {
				continue
			}
			upsertSetting(key, v)
		}
	} else {
		args := c.Request().PostArgs()
		args.VisitAll(func(key, value []byte) {
			k, val := string(key), string(value)
			v, ok := validateSetting(k, val)
			if !ok {
				return
			}
			upsertSetting(k, v)
		})
	}

	// Only reset unchecked booleans when submitting the full settings modal.
	// The inline title form also POSTs here but only sends dashboard_title/subtitle,
	// so we must not reset bools in that case. theme_mode is a reliable indicator
	// that the full settings form was submitted.
	if submitted["theme_mode"] {
		for _, key := range boolSettings {
			if !submitted[key] {
				database.DB.Model(&models.Setting{}).
					Where("dashboard_id = ? AND key = ?", dashID, key).
					Updates(map[string]interface{}{"value": "false", "updated_at": time.Now()})
			}
		}

		// Persist theme_mode globally in the session so it is shared across all dashboards.
		themeVal := strings.TrimSpace(c.FormValue("theme_mode"))
		if themeVal == "dark" || themeVal == "light" {
			if sess, sessErr := Store.Get(c); sessErr == nil {
				sess.Set("theme_mode", themeVal)
				if saveErr := sess.Save(); saveErr != nil {
					log.Printf("SaveSettingsHandler: failed to save theme in session: %v", saveErr)
				}
			}
		}
	}

	return c.SendStatus(200)
}

func GetSettingsHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}

	var settingsList []models.Setting
	if err := database.DB.Where("dashboard_id = ?", dashID).Find(&settingsList).Error; err != nil {
		log.Printf("settings: failed to fetch settings: %v", err)
	}
	settings := make(map[string]string)
	for _, s := range settingsList {
		settings[s.Key] = s.Value
	}
	// theme_mode is global: read from session if set.
	if sess, sessErr := Store.Get(c); sessErr == nil {
		if t, ok := sess.Get("theme_mode").(string); ok && (t == "dark" || t == "light") {
			settings["theme_mode"] = t
		}
	}

	// Seed defaults so sliders always have a value.
	if settings["grid_cell"] == "" {
		settings["grid_cell"] = "110"
	}
	if settings["grid_gap"] == "" {
		settings["grid_gap"] = "16"
	}
	if settings["show_cpu_widget"] == "" {
		settings["show_cpu_widget"] = "true"
	}
	if settings["show_ram_widget"] == "" {
		settings["show_ram_widget"] = "true"
	}
	return c.Render("partials/settings_modal", fiber.Map{
		"Settings": settings,
	})
}

func ReorderHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}

	args := c.Request().PostArgs()
	items := args.PeekMulti("item")
	for i, item := range items {
		id, err := strconv.Atoi(string(item))
		if err == nil {
			if res := database.DB.Model(&models.Link{}).
				Where("id = ? AND dashboard_id = ?", id, dashID).
				Update("order_index", i); res.Error != nil {
				log.Printf("reorder: failed to update order for link %d: %v", id, res.Error)
			}
		}
	}
	return c.SendStatus(200)
}

// SaveLayoutHandler persists grid position/size for all links at once.
// Expects JSON body: [{"id":1,"grid_col":1,"grid_row":1,"col_span":2,"row_span":1}, ...]
func SaveLayoutHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}

	type Item struct {
		ID      uint `json:"id"`
		GridCol int  `json:"grid_col"`
		GridRow int  `json:"grid_row"`
		ColSpan int  `json:"col_span"`
		RowSpan int  `json:"row_span"`
	}
	var items []Item
	if err := c.BodyParser(&items); err != nil {
		return c.Status(400).SendString("Invalid body")
	}
	for _, item := range items {
		if item.ColSpan < 1 {
			item.ColSpan = 1
		}
		if item.RowSpan < 1 {
			item.RowSpan = 1
		}
		if item.GridCol < 1 {
			item.GridCol = 1
		}
		if item.GridRow < 1 {
			item.GridRow = 1
		}
		if res := database.DB.Model(&models.Link{}).
			Where("id = ? AND dashboard_id = ?", item.ID, dashID).
			Updates(map[string]interface{}{
				"grid_col": item.GridCol,
				"grid_row": item.GridRow,
				"col_span": item.ColSpan,
				"row_span": item.RowSpan,
			}); res.Error != nil {
			log.Printf("save-layout: failed to update layout for link %d: %v", item.ID, res.Error)
		}
	}
	return c.SendStatus(200)
}

func DeleteLinksHandler(c *fiber.Ctx) error {
	dashID, err := getCurrentDashboardID(c)
	if err != nil {
		return c.Status(500).SendString("Error loading dashboard")
	}

	args := c.Request().PostArgs()
	items := args.PeekMulti("selected_links")

	if len(items) == 0 {
		return c.Status(400).SendString("No links selected")
	}

	for _, item := range items {
		id, err := strconv.Atoi(string(item))
		if err != nil {
			continue
		}
		var link models.Link
		if err := database.DB.Where("id = ? AND dashboard_id = ?", id, dashID).First(&link).Error; err == nil {
			if strings.HasPrefix(link.Icon, "/uploads/") {
				if err := os.Remove(filepath.Join("./uploads", filepath.Base(link.Icon))); err != nil && !os.IsNotExist(err) {
					log.Printf("delete-links: failed to remove icon for link %d: %v", id, err)
				}
			}
			if res := database.DB.Delete(&models.Link{}, id); res.Error != nil {
				log.Printf("delete-links: failed to delete link %d: %v", id, res.Error)
			}
		}
	}

	var links []models.Link
	database.DB.Where("dashboard_id = ?", dashID).Order("order_index asc, id asc").Find(&links)
	return c.Render("partials/link_list", links)
}
