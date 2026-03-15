package middleware

import (
	"strings"

	"volt/internal/database"
	"volt/internal/handlers"
	"volt/internal/models"

	"github.com/gofiber/fiber/v2"
)

func AuthMiddleware(c *fiber.Ctx) error {
	path := c.Path()
	if path == "/login" || path == "/setup" || path == "/logout" {
		return c.Next()
	}

	var count int64
	database.DB.Model(&models.User{}).Count(&count)
	if count == 0 {
		return htmxAwareRedirect(c, "/setup")
	}

	sess, err := handlers.Store.Get(c)
	if err != nil {
		return htmxAwareRedirect(c, "/login")
	}

	if sess.Get("user_id") == nil {
		return htmxAwareRedirect(c, "/login")
	}

	return c.Next()
}

// htmxAwareRedirect redirects the user to the given URL. When the request
// originates from HTMX or is a fetch/XHR API call it responds with 401 so
// the client-side handler can perform a full-page navigation to the login
// screen instead of silently swapping in partial HTML.
func htmxAwareRedirect(c *fiber.Ctx, url string) error {
	// HTMX requests: use HX-Redirect so HTMX does a full navigation
	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", url)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	// Fetch/XHR API calls (e.g. /api/stats, /save-layout): return 401
	// so the JS fetch handler can redirect to login.
	// Only treat as an API call when the path is under /api/ or the request
	// explicitly signals XHR — never rely on Accept header matching because
	// browsers send Accept: */* which would match application/json.
	if c.Get("X-Requested-With") == "XMLHttpRequest" ||
		strings.HasPrefix(c.Path(), "/api/") {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	return c.Redirect(url)
}
