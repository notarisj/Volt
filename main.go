package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"

	"volt/internal/database"
	"volt/internal/handlers"
	"volt/internal/middleware"
)

func main() {
	engine := html.New("./views", ".html")

	app := fiber.New(fiber.Config{
		Views: engine,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if e, ok := err.(*fiber.Error); ok {
				return c.Status(e.Code).SendString(e.Message)
			}
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		},
	})

	app.Use(logger.New())
	app.Use(recover.New())

	// Security headers
	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' https://unpkg.com https://cdn.tailwindcss.com; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src 'self' https://fonts.gstatic.com; "+
				"img-src 'self' data: blob: https://www.google.com https://fonts.gstatic.com; "+
				"connect-src 'self'; "+
				"frame-ancestors 'none';")
		return c.Next()
	})

	database.Init()

	if err := os.MkdirAll("./uploads", 0755); err != nil {
		log.Fatalf("Failed to create uploads directory: %v", err)
	}

	app.Static("/static", "./static")
	app.Static("/uploads", "./uploads")

	// Rate limiter for auth mutation endpoints — limits brute-force attempts
	authLimiter := limiter.New(limiter.Config{
		Max:        10,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).SendString("Too many attempts. Please try again later.")
		},
	})

	// Auth routes
	app.Get("/setup", handlers.GetSetupHandler)
	app.Post("/setup", authLimiter, handlers.PostSetupHandler)
	app.Get("/login", handlers.GetLoginHandler)
	app.Post("/login", authLimiter, handlers.PostLoginHandler)
	app.Get("/logout", handlers.LogoutHandler)

	app.Use(middleware.AuthMiddleware)

	// Protected routes
	app.Get("/", handlers.IndexHandler)

	// Dashboard management
	app.Get("/dashboards/panel", handlers.GetDashboardPanelHandler)
	app.Post("/dashboards", handlers.CreateDashboardHandler)
	app.Post("/dashboards/switch/:id", handlers.SwitchDashboardHandler)
	app.Post("/dashboards/:id/rename", handlers.RenameDashboardHandler)
	app.Delete("/dashboards/:id", handlers.DeleteDashboardHandler)

	app.Post("/add-link", handlers.AddLinkHandler)
	app.Get("/edit-link/:id", handlers.GetEditLinkHandler)
	app.Post("/edit-link/:id", handlers.EditLinkHandler)
	app.Get("/delete-link/:id", handlers.GetDeleteLinkHandler)
	app.Get("/settings", handlers.GetSettingsHandler)
	app.Post("/edit-settings", handlers.SaveSettingsHandler)
	app.Post("/reorder", handlers.ReorderHandler)
	app.Post("/save-layout", handlers.SaveLayoutHandler)
	app.Post("/delete-links", handlers.DeleteLinksHandler)
	app.Delete("/remove-link/:id", handlers.RemoveLinkHandler)
	app.Get("/api/stats", handlers.StatsHandler)
	app.Get("/api/stats/stream", handlers.StatsStreamHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8686"
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("Shutting down server...")
		if err := app.Shutdown(); err != nil {
			log.Fatalf("Server forced shutdown: %v", err)
		}
	}()

	if err := app.Listen(":" + port); err != nil {
		log.Println("Server stopped:", err)
	}
}
