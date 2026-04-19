package api

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// --- Prompts ---

func (h *Handler) ListPrompts(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	items, err := h.repo.ListPromptsByBusiness(bid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

func (h *Handler) CreatePrompt(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	
	var p model.ProjectPrompt
	if err := c.BodyParser(&p); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	p.BusinessID = bid
	
	if err := h.repo.CreatePrompt(&p); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(p)
}

// --- Sales ---

func (h *Handler) ListSales(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	items, err := h.repo.ListSalesByBusiness(bid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

func (h *Handler) UpsertSales(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	
	var s model.SalesAssignment
	if err := c.BodyParser(&s); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	s.BusinessID = bid
	
	if err := h.repo.UpsertSalesAssignment(&s); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(s)
}

func (h *Handler) ToggleSales(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.repo.ToggleSalesActive(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// --- Leads ---

func (h *Handler) ListLeadsExtended(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	search := c.Query("search", "")
	
	leads, total, err := h.repo.ListLeadsExtended(bid, page, limit, search)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	
	return c.JSON(fiber.Map{
		"items": leads,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *Handler) ListMessages(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid lead id"})
	}
	items, err := h.repo.ListMessagesByLead(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

// --- Users ---

func (h *Handler) ListUsers(c *fiber.Ctx) error {
	items, err := h.repo.ListUsers()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

type createUserReq struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) CreateUser(c *fiber.Ctx) error {
	var req createUserReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
	}

	user := model.User{
		ID:           uuid.New(),
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hash),
	}

	if err := h.repo.CreateUser(&user); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(user)
}

func (h *Handler) DeleteUser(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	if err := h.repo.DeleteUser(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}
