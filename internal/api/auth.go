package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var req loginReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	user, err := h.repo.GetUserByEmail(req.Email)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized: Invalid credentials"})
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized: Invalid credentials"})
	}

	// Create JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID.String(),
		"name":    user.Name,
		"email":   user.Email,
		"exp":     time.Now().Add(time.Hour * 72).Unix(),
	})

	t, err := token.SignedString(h.jwtSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to sign token"})
	}

	return c.JSON(fiber.Map{
		"token": t,
		"user": fiber.Map{
			"id":    user.ID,
			"name":  user.Name,
			"email": user.Email,
		},
	})
}
