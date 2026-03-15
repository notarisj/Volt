package handlers

import (
	"log"
	"strings"
	"time"
	"volt/internal/database"
	"volt/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	"golang.org/x/crypto/bcrypt"
)

func getThemeMode() string {
	var dash models.Dashboard
	if err := database.DB.Order("created_at asc, id asc").First(&dash).Error; err != nil {
		return "dark"
	}
	var s models.Setting
	if err := database.DB.Where("dashboard_id = ? AND key = ?", dash.ID, "theme_mode").First(&s).Error; err != nil {
		return "dark"
	}
	if s.Value == "light" {
		return "light"
	}
	return "dark"
}

var Store = session.New(session.Config{
	Expiration:     24 * time.Hour,
	CookieSameSite: "Strict",
	CookieHTTPOnly: true,
})

func GetSetupHandler(c *fiber.Ctx) error {
	var count int64
	database.DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		return c.Redirect("/login")
	}
	return c.Render("auth/setup", fiber.Map{})
}

func PostSetupHandler(c *fiber.Ctx) error {
	var count int64
	database.DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		return c.Redirect("/login")
	}

	username := strings.TrimSpace(c.FormValue("username"))
	password := c.FormValue("password")

	if username == "" || password == "" {
		return c.Render("auth/setup", fiber.Map{"Error": "Username and password are required"})
	}

	if len(username) > 64 {
		return c.Render("auth/setup", fiber.Map{"Error": "Username must be 64 characters or fewer"})
	}

	if len(password) < 8 {
		return c.Render("auth/setup", fiber.Map{"Error": "Password must be at least 8 characters"})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).SendString("Error hashing password")
	}

	user := models.User{
		Username: username,
		Password: string(hashedPassword),
	}

	if err := database.DB.Create(&user).Error; err != nil {
		return c.Render("auth/setup", fiber.Map{"Error": "Error creating user"})
	}

	sess, err := Store.Get(c)
	if err != nil {
		return c.Status(500).SendString("Session error")
	}
	sess.Set("user_id", user.ID)
	sess.Set("theme_mode", getThemeMode())
	if err := sess.Save(); err != nil {
		return c.Status(500).SendString("Session save error")
	}

	return c.Redirect("/")
}

func GetLoginHandler(c *fiber.Ctx) error {
	var count int64
	database.DB.Model(&models.User{}).Count(&count)
	if count == 0 {
		return c.Redirect("/setup")
	}
	return c.Render("auth/login", fiber.Map{"Theme": getThemeMode()})
}

func PostLoginHandler(c *fiber.Ctx) error {
	username := strings.TrimSpace(c.FormValue("username"))
	password := c.FormValue("password")

	var user models.User
	if err := database.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return c.Render("auth/login", fiber.Map{"Error": "Invalid credentials", "Theme": getThemeMode()})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return c.Render("auth/login", fiber.Map{"Error": "Invalid credentials", "Theme": getThemeMode()})
	}

	sess, err := Store.Get(c)
	if err != nil {
		log.Println("Session get error:", err)
		return c.Status(500).SendString("Session error")
	}
	sess.Set("user_id", user.ID)
	sess.Set("theme_mode", getThemeMode())
	if err := sess.Save(); err != nil {
		log.Println("Session save error:", err)
		return c.Status(500).SendString("Session save error")
	}

	return c.Redirect("/")
}

func LogoutHandler(c *fiber.Ctx) error {
	sess, err := Store.Get(c)
	if err != nil {
		return c.Redirect("/login")
	}
	if err := sess.Destroy(); err != nil {
		return c.Status(500).SendString("Logout error")
	}
	return c.Redirect("/login")
}
