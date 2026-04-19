package api

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/workflow-builder/core/internal/integrations/gsheets"
	"github.com/workflow-builder/core/internal/integrations/retell"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/pkg/crypto"
)

// Handler holds dependencies for all HTTP handlers
type Handler struct {
	repo      *Repo
	asynq     *asynq.Client
	rdb       *redis.Client
	encKey    []byte
	jwtSecret []byte
}

func NewHandler(repo *Repo, asynqClient *asynq.Client, rdb *redis.Client, encKey, jwtSecret []byte) *Handler {
	return &Handler{repo: repo, asynq: asynqClient, rdb: rdb, encKey: encKey, jwtSecret: jwtSecret}
}

// RegisterRoutes wires all routes
func (h *Handler) RegisterRoutes(app *fiber.App) {
	// 1. Auth & Public (No Auth Middleware)
	auth := app.Group("/api/auth")
	auth.Post("/login", h.Login)

	// Public Webhooks (Internal security check per handler)
	public := app.Group("/api/webhooks")
	public.Post("/leadflow/retell", h.HandleLeadflowRetell)
	public.Post("/leadflow/chat-inbound/:slug", h.HandleChatInbound)
	public.All("/*", h.HandleWebhook)

	// 2. Dashboard & Management (Protected by JWT or API Key)
	mgmt := app.Group("/api", AdminAuth(h.jwtSecret))

	// Business & Project Management
	mgmt.Get("/businesses", h.ListBusinesses)
	mgmt.Post("/businesses", h.CreateBusiness)
	mgmt.Get("/businesses/:id", h.GetBusiness)
	mgmt.Put("/businesses/:id", h.UpdateBusiness)
	mgmt.Delete("/businesses/:id", h.DeleteBusiness)

	// Audit Logs
	mgmt.Get("/businesses/:bid/audit-logs", h.ListAuditLogs)

	// Registry
	mgmt.Get("/registry", h.GetRegistry)

	// Workflows
	mgmt.Get("/businesses/:bid/workflows", h.ListWorkflows)
	mgmt.Post("/businesses/:bid/workflows", h.CreateWorkflow)
	mgmt.Patch("/workflows/:id/toggle", h.ToggleWorkflow)
	mgmt.Delete("/workflows/:id", h.DeleteWorkflowHandler)
	mgmt.Post("/workflows/:id/trigger", h.TriggerWorkflow)
	mgmt.Post("/workflows/:id/stop", h.StopWorkflow)
	mgmt.Get("/executions/:id", h.GetExecutionStatus)
	mgmt.Get("/workflows/:id/executions", h.ListExecutions)
	mgmt.Patch("/workflows/:id/vars", h.UpdateWorkflowVars)
	mgmt.Patch("/workflows/:id/cron", h.UpdateWorkflowCron)
	mgmt.Patch("/workflows/:id/stop-time", h.UpdateWorkflowStopTime)

	// Credentials
	mgmt.Get("/businesses/:bid/credentials", h.ListCredentials)
	mgmt.Post("/credentials", h.CreateCredential)
	mgmt.Delete("/credentials/:id", h.DeleteCredential)

	// Prompts & Sales
	mgmt.Get("/businesses/:bid/prompts", h.ListPrompts)
	mgmt.Post("/prompts", h.CreatePrompt)
	mgmt.Get("/businesses/:bid/sales", h.ListSales)
	mgmt.Post("/sales", h.UpsertSales)
	mgmt.Patch("/sales/:id/toggle", h.ToggleSales)

	// Leads
	mgmt.Get("/businesses/:bid/leads", h.ListLeadsExtended)
	mgmt.Get("/leads/:id/messages", h.ListMessages)

	// Executions by business
	mgmt.Get("/businesses/:bid/executions", h.ListExecutionsByBusiness)
	mgmt.Get("/executions/:id/logs", h.ListExecutionLogs)

	// Users
	mgmt.Get("/users", h.ListUsers)
	mgmt.Post("/users", h.CreateUser)
	mgmt.Delete("/users/:id", h.DeleteUser)
}

func (h *Handler) HandleLeadflowRetell(c *fiber.Ctx) error {
	// Implemented in webhooks_leadflow.go
	return h.leadflowRetell(c)
}

func (h *Handler) HandleChatInbound(c *fiber.Ctx) error {
	// Implemented in webhooks_leadflow.go
	return h.chatInbound(c)
}

// ==================== Registry ====================

func (h *Handler) GetRegistry(c *fiber.Ctx) error {
	type enriched struct {
		Signature string      `json:"signature"`
		Name      string      `json:"name"`
		Desc      string      `json:"description"`
		Category  string      `json:"category,omitempty"`
		Params    []sdk.Param `json:"params"`
		Steps     []sdk.Step  `json:"steps"`
	}
	res := []enriched{}
	for sig, def := range sdk.Registry.Workflows {
		res = append(res, enriched{
			Signature: sig,
			Name:      def.Name,
			Desc:      def.Description,
			Category:  def.Category,
			Params:    def.Params,
			Steps:     def.Steps,
		})
	}
	return c.JSON(res)
}

// ==================== Businesses ====================

func (h *Handler) ListBusinesses(c *fiber.Ctx) error {
	items, err := h.repo.ListBusinesses()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

type createBusinessReq struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func (h *Handler) CreateBusiness(c *fiber.Ctx) error {
	var req createBusinessReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	if req.Kind == "" {
		req.Kind = "standard"
	}
	slug := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))

	// Check if slug exists to avoid 500 error from unique constraint
	existing, _ := h.repo.GetBusinessBySlug(slug)
	if existing != nil && existing.ID != uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "A business with this name already exists. Please choose a different name."})
	}

	b := model.Business{Name: req.Name, Slug: slug, Kind: req.Kind}
	if err := h.repo.CreateBusiness(&b); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(b)
}

func (h *Handler) DeleteBusiness(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.repo.DeleteBusiness(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ==================== Workflows ====================

func (h *Handler) ListWorkflows(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	items, err := h.repo.ListWorkflows(bid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	type enriched struct {
		model.Workflow
		SDKName  string      `json:"sdk_name"`
		SDKDesc  string      `json:"sdk_description"`
		Category string      `json:"category,omitempty"`
		Params   []sdk.Param `json:"params"`
		Steps    []sdk.Step  `json:"steps"`
	}
	result := []enriched{}
	for _, w := range items {
		e := enriched{Workflow: w}
		if def, ok := sdk.Registry.Workflows[w.Signature]; ok {
			e.SDKName = def.Name
			e.SDKDesc = def.Description
			e.Category = def.Category
			e.Params = def.Params
			e.Steps = def.Steps
		}
		result = append(result, e)
	}
	return c.JSON(result)
}

type createWorkflowReq struct {
	Signature   string `json:"signature"`
	Alias       string `json:"alias"`
	TriggerCron string `json:"trigger_cron"`
}

func (h *Handler) CreateWorkflow(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	var req createWorkflowReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.Signature == "" || req.Alias == "" {
		return c.Status(400).JSON(fiber.Map{"error": "signature and alias required"})
	}
	if _, ok := sdk.Registry.Workflows[req.Signature]; !ok {
		return c.Status(400).JSON(fiber.Map{"error": "unknown workflow signature"})
	}
	w := model.Workflow{
		BusinessID:  bid,
		Signature:   req.Signature,
		Alias:       req.Alias,
		TriggerCron: req.TriggerCron,
	}
	if err := h.repo.CreateWorkflow(&w); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(w)
}

func (h *Handler) ToggleWorkflow(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	w, err := h.repo.ToggleWorkflow(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(w)
}

func (h *Handler) UpdateWorkflowCron(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	type body struct {
		TriggerCron string `json:"trigger_cron"`
	}
	var b body
	if err := c.BodyParser(&b); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if err := h.repo.UpdateWorkflowCron(id, b.TriggerCron); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) UpdateWorkflowStopTime(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	type body struct {
		StopTime string `json:"stop_time"`
	}
	var b body
	if err := c.BodyParser(&b); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if err := h.repo.UpdateWorkflowStopTime(id, b.StopTime); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) UpdateWorkflowVars(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	type body struct {
		Variables string `json:"variables"`
	}
	var b body
	if err := c.BodyParser(&b); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if err := h.repo.UpdateWorkflowVars(id, b.Variables); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) DeleteWorkflowHandler(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.repo.DeleteWorkflow(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ==================== Trigger / Stop ====================

func (h *Handler) TriggerWorkflow(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	wf, err := h.repo.GetWorkflow(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "workflow not found"})
	}

	// Safety Check: Restricted Operating Hours for N8NTriggerWorkflow (8 AM - 6 PM Jakarta)
	if wf.Signature == "N8NTriggerWorkflow" {
		loc := time.FixedZone("Asia/Jakarta", 7*3600)
		now := time.Now().In(loc)
		if now.Hour() < 8 || now.Hour() >= 18 {
			return c.Status(403).JSON(fiber.Map{
				"error": "Restricted Operating Hours: This workflow is restricted to 8:00 AM - 6:00 PM Jakarta time. Executes outside this window are automatically rejected.",
			})
		}
	}

	// Create execution record
	uidStr, _ := c.Locals("user_id").(string)
	var uid *uuid.UUID
	if parsed, err := uuid.Parse(uidStr); err == nil {
		uid = &parsed
	}

	exec := model.Execution{
		ID:              uuid.New(),
		WorkflowID:      wf.ID,
		Status:          "queued",
		TriggeredByID:   uid,
		TriggeredByType: "user",
	}
	if err := h.repo.CreateExecution(&exec); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Create Audit Log
	h.repo.CreateAuditLog(&model.AuditLog{
		BusinessID: wf.BusinessID,
		UserID:     uid,
		Action:     "START_WORKFLOW",
		TargetID:   exec.ID,
		TargetType: "execution",
		Details:    `{"workflow_alias": "` + wf.Alias + `"}`,
	})

	// Enqueue task in Redis via Asynq
	payload, _ := json.Marshal(map[string]string{
		"workflow_id":  wf.ID.String(),
		"execution_id": exec.ID.String(),
	})
	task := asynq.NewTask("workflow:execute", payload)
	_, err = h.asynq.Enqueue(task, asynq.Queue("executions"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to enqueue: " + err.Error()})
	}

	return c.Status(201).JSON(exec)
}

func (h *Handler) StopWorkflow(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	// Find the latest running execution for this workflow
	var exec model.Execution
	err = h.repo.db.Preload("Workflow").Where("workflow_id = ? AND status IN ?", id, []string{"queued", "running"}).
		Order("created_at desc").First(&exec).Error
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "no running execution found"})
	}

	// Set stop flag in Redis (this is what n8n will check)
	h.rdb.Set(c.Context(), "stop:"+exec.ID.String(), "1", 30*time.Minute)

	// Update local status so UI reflects stopping
	h.repo.db.Model(&exec).Update("status", "stopped")

	// Create Audit Log
	uidStr, _ := c.Locals("user_id").(string)
	var uid *uuid.UUID
	if parsed, err := uuid.Parse(uidStr); err == nil {
		uid = &parsed
	}
	h.repo.CreateAuditLog(&model.AuditLog{
		BusinessID: exec.Workflow.BusinessID,
		UserID:     uid,
		Action:     "STOP_WORKFLOW",
		TargetID:   exec.ID,
		TargetType: "execution",
		Details:    `{"workflow_alias": "` + exec.Workflow.Alias + `"}`,
	})

	return c.JSON(fiber.Map{"ok": true, "execution_id": exec.ID})
}

func (h *Handler) ListAuditLogs(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	items, err := h.repo.ListAuditLogs(bid, 50)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	// Fetch users for attribution
	users, _ := h.repo.ListUsers()
	userMap := make(map[uuid.UUID]string)
	for _, u := range users {
		userMap[u.ID] = u.Name
	}

	type enrichedAudit struct {
		model.AuditLog
		UserName      string `json:"user_name,omitempty"`
		WorkflowAlias string `json:"workflow_alias,omitempty"`
	}

	result := []enrichedAudit{}
	for _, log := range items {
		e := enrichedAudit{AuditLog: log}
		if log.UserID != nil {
			e.UserName = userMap[*log.UserID]
		}
		
		// Parse details for workflow alias since the UI expects it as a top-level field
		var details map[string]interface{}
		if err := json.Unmarshal([]byte(log.Details), &details); err == nil {
			if alias, ok := details["workflow_alias"].(string); ok {
				e.WorkflowAlias = alias
			}
		}
		result = append(result, e)
	}

	return c.JSON(result)
}

func (h *Handler) GetExecutionStatus(c *fiber.Ctx) error {
	id := c.Params("id")

	// Check Redis stop flag first
	val, _ := h.rdb.Get(c.Context(), "stop:"+id).Result()
	if val == "1" {
		log.Printf("[DEBUG] StatusCheck: ID %s flagged as STOP in Redis", id)
		return c.JSON(fiber.Map{"status": "stop"})
	}

	var exec model.Execution
	if err := h.repo.db.First(&exec, "id = ?", id).Error; err != nil {
		log.Printf("[DEBUG] StatusCheck: ID %s NOT FOUND in DB: %v", id, err)
		return c.JSON(fiber.Map{"status": "stop"})
	}

	log.Printf("[DEBUG] StatusCheck: ID %s found with status %s", id, exec.Status)

	if exec.Status == "stopped" || exec.Status == "completed" || exec.Status == "failed" {
		return c.JSON(fiber.Map{"status": "stop"})
	}

	return c.JSON(fiber.Map{"status": "running"})
}

// ==================== Credentials ====================

type n8nCallbackReq struct {
	CoreExecutionID uuid.UUID `json:"core_execution_id"`
	N8NExecutionID  string    `json:"n8n_execution_id"`
}

func (h *Handler) HandleN8NCallback(c *fiber.Ctx) error {
	var req n8nCallbackReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	if req.CoreExecutionID == uuid.Nil || req.N8NExecutionID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "core_execution_id and n8n_execution_id required"})
	}

	if err := h.repo.db.Model(&model.Execution{}).Where("id = ?", req.CoreExecutionID).Update("external_id", req.N8NExecutionID).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// ==================== Credentials ====================

func (h *Handler) ListCredentials(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	items, err := h.repo.ListCredentials(bid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

type createCredentialReq struct {
	Label       string `json:"label"`
	Integration string `json:"integration"`
	Data        string `json:"data"`
	IsGlobal    bool   `json:"is_global"`
}

func (h *Handler) CreateCredential(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	var req createCredentialReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.Label == "" || req.Integration == "" || req.Data == "" {
		return c.Status(400).JSON(fiber.Map{"error": "label, integration, and data required"})
	}

	// Encrypt the secret value
	encrypted, err := crypto.Encrypt(h.encKey, []byte(req.Data))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "encryption failed"})
	}

	cred := model.Credential{
		BusinessID:  bid,
		Label:       req.Label,
		Integration: req.Integration,
		IsGlobal:    req.IsGlobal,
		DataEnc:     encrypted,
	}
	if err := h.repo.CreateCredential(&cred); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(cred)
}

func (h *Handler) VerifyCredential(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	var cred model.Credential
	if err := h.repo.db.First(&cred, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "credential not found"})
	}

	// Decrypt
	val, err := crypto.Decrypt(h.encKey, cred.DataEnc)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to decrypt"})
	}

	ctx := c.Context()
	switch cred.Integration {
	case "retell_ai":
		client := retell.NewClient(string(val))
		if err := client.Verify(ctx); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	case "google_sheets":
		client, err := gsheets.NewClient(ctx, string(val))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON: " + err.Error()})
		}
		if err := client.Verify(ctx); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	default:
		return c.Status(400).JSON(fiber.Map{"error": "verification not supported for " + cred.Integration})
	}

	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) DeleteCredential(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.repo.DeleteCredential(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ==================== Executions ====================

func (h *Handler) ListExecutions(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	items, err := h.repo.ListExecutions(id, 50)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

func (h *Handler) ListExecutionsByBusiness(c *fiber.Ctx) error {
	bid, err := uuid.Parse(c.Params("bid"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid business id"})
	}
	items, err := h.repo.ListExecutionsByBusiness(bid, 50)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

func (h *Handler) ListExecutionLogs(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	items, err := h.repo.ListExecutionLogs(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(items)
}

func (h *Handler) PreviewCredentialData(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	type body struct {
		SheetID string `json:"sheet_id"`
		TabName string `json:"tab_name"`
	}
	var b body
	if err := c.BodyParser(&b); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	cred, err := h.repo.GetCredential(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "credential not found"})
	}

	val, err := crypto.Decrypt(h.encKey, cred.DataEnc)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "decryption failed"})
	}

	switch cred.Integration {
	case "google_sheets":
		client, err := gsheets.NewClient(c.Context(), string(val))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "client error: " + err.Error()})
		}
		// Read 10 rows + 1 for header
		rows, err := client.ReadRows(c.Context(), b.SheetID, b.TabName+"!A1:Z11")
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "fetch error: " + err.Error()})
		}

		var data []map[string]string
		for _, r := range rows {
			data = append(data, r.ToMap())
		}
		return c.JSON(data)

	default:
		return c.Status(400).JSON(fiber.Map{"error": "preview not supported for this integration"})
	}
}

func (h *Handler) GetWorkflowLogic(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	wf, err := h.repo.GetWorkflow(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "workflow not found"})
	}

	// Map signature to folder
	folder := ""
	switch wf.Signature {
	case "MortgageCallWorkflow":
		folder = "mortgage"
	default:
		return c.Status(400).JSON(fiber.Map{"error": "logic not documented for this workflow"})
	}

	// Resolve absolute path to ensure we find the file regardless of CWD
	wd, _ := os.Getwd()
	fPath := filepath.Join(wd, "internal", "workflows", folder, "LOGIC.md")

	content, err := os.ReadFile(fPath)
	if err != nil {
		// Fallback to relative if absolute fails for some reason
		fPath = filepath.Join("internal", "workflows", folder, "LOGIC.md")
		content, err = os.ReadFile(fPath)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error":   "logic file not found",
				"path":    fPath,
				"details": err.Error(),
			})
		}
	}

	return c.JSON(fiber.Map{"content": string(content)})
}

// Placeholder stubs for project/execution management
func (h *Handler) GetBusiness(c *fiber.Ctx) error    { return c.SendStatus(501) }
func (h *Handler) UpdateBusiness(c *fiber.Ctx) error { return c.SendStatus(501) }
func (h *Handler) GetExecution(c *fiber.Ctx) error   { return c.SendStatus(501) }

// ==================== Webhooks ====================

func (h *Handler) HandleWebhook(c *fiber.Ctx) error {
	path := c.Params("*")

	// Find matching webhook handler in SDK registry
	for _, def := range sdk.Registry.Webhooks {
		// Match by path suffix (e.g. "/callbacks/retell")
		if strings.TrimPrefix(def.Path, "/") == path {
			body := c.Body()
			query := make(map[string]string)
			c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
				query[string(key)] = string(value)
			})

			if err := def.Func(c.Context(), h.repo.db, h.rdb, h.encKey, query, body); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			return c.JSON(fiber.Map{"ok": true})
		}
	}

	return c.Status(404).JSON(fiber.Map{"error": "webhook not found"})
}

// ==================== Helpers (unused imports) ====================
var _ = io.ReadAll // keep io import for webhook body reading
