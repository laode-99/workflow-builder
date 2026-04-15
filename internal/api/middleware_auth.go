package api

import (
	"os"

	"github.com/gofiber/fiber/v2"
)

// AdminAuth is a middleware that protects routes with a static API Key.
// It checks the 'X-API-Key' header.
func AdminAuth() fiber.Handler {
	apiKey := os.Getenv("ADMIN_API_KEY")
	if apiKey == "" {
		// Default to a development key if not set, but warn.
		apiKey = "admin-secret-dev"
	}

	return func(c *fiber.Ctx) error {
		key := c.Get("X-API-Key")
		if key != apiKey {
			return c.Status(401).JSON(fiber.Map{
				"error": "Unauthorized: Invalid or missing API Key",
			})
		}
		return c.Next()
	}
}

// ProjectAuth is a placeholder for project-specific token verification.
// For now, it delegates to AdminAuth or allows all if Admin key is present.
func ProjectAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// In Phase 2, we would verify per-project tokens.
		// For MVP, we use the global Admin key check.
		return AdminAuth()(c)
	}
}
