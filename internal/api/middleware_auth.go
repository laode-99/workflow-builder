package api

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// AdminAuth is a middleware that protects routes with either a JWT or a static API Key.
func AdminAuth(jwtSecret []byte) fiber.Handler {
	apiKey := os.Getenv("ADMIN_API_KEY")
	if apiKey == "" {
		apiKey = "admin-secret-dev"
	}

	return func(c *fiber.Ctx) error {
		// 0. Skip auth for public paths
		path := c.Path()
		if strings.HasPrefix(path, "/api/auth") || strings.HasPrefix(path, "/api/webhooks") {
			return c.Next()
		}

		// 1. Check for modern Authorization: Bearer <token>
		authHeader := c.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				return jwtSecret, nil
			})

			if err == nil && token.Valid {
				if claims, ok := token.Claims.(jwt.MapClaims); ok {
					c.Locals("user_id", claims["user_id"])
					c.Locals("user_name", claims["name"])
					return c.Next()
				}
			}
		}

		// 2. Fallback to legacy X-API-Key for scripts/legacy clients
		key := c.Get("X-API-Key")
		if key != "" && key == apiKey {
			c.Locals("user_id", "system-api-key")
			c.Locals("user_name", "Legacy Key")
			return c.Next()
		}

		return c.Status(401).JSON(fiber.Map{
			"error": "Unauthorized: Invalid or missing authentication",
		})
	}
}

// ProjectAuth is a placeholder for project-specific token verification.
// For now, it delegates to AdminAuth.
func ProjectAuth(jwtSecret []byte) fiber.Handler {
	return AdminAuth(jwtSecret)
}
