package api

import (
	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"gorm.io/gorm"
)

// Repo wraps GORM for clean data access. No business logic here.
type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// --- Businesses ---

func (r *Repo) ListBusinesses() ([]model.Business, error) {
	var items []model.Business
	err := r.db.Order("created_at desc").Find(&items).Error
	return items, err
}

func (r *Repo) CreateBusiness(b *model.Business) error {
	return r.db.Create(b).Error
}

func (r *Repo) GetBusiness(id uuid.UUID) (*model.Business, error) {
	var b model.Business
	err := r.db.First(&b, "id = ?", id).Error
	return &b, err
}

func (r *Repo) GetBusinessBySlug(slug string) (*model.Business, error) {
	var b model.Business
	err := r.db.First(&b, "slug = ?", slug).Error
	return &b, err
}

func (r *Repo) DeleteBusiness(id uuid.UUID) error {
	// Cascade: delete executions → workflows → credentials → business
	var wfIDs []uuid.UUID
	r.db.Model(&model.Workflow{}).Where("business_id = ?", id).Pluck("id", &wfIDs)
	if len(wfIDs) > 0 {
		r.db.Where("workflow_id IN ?", wfIDs).Delete(&model.Execution{})
	}
	r.db.Where("business_id = ?", id).Delete(&model.Workflow{})
	r.db.Where("business_id = ?", id).Delete(&model.Credential{})
	return r.db.Delete(&model.Business{}, "id = ?", id).Error
}

// --- Workflows ---

func (r *Repo) ListWorkflowsByBusiness(businessID uuid.UUID) ([]model.Workflow, error) {
	var items []model.Workflow
	err := r.db.Where("business_id = ?", businessID).Order("created_at desc").Find(&items).Error
	return items, err
}

func (r *Repo) CreateWorkflow(w *model.Workflow) error {
	return r.db.Create(w).Error
}

func (r *Repo) GetWorkflow(id uuid.UUID) (*model.Workflow, error) {
	var w model.Workflow
	err := r.db.First(&w, "id = ?", id).Error
	return &w, err
}

func (r *Repo) DeleteWorkflow(id uuid.UUID) error {
	// Delete executions first, then workflow
	r.db.Where("workflow_id = ?", id).Delete(&model.Execution{})
	return r.db.Delete(&model.Workflow{}, "id = ?", id).Error
}

func (r *Repo) ToggleWorkflow(id uuid.UUID) (*model.Workflow, error) {
	var w model.Workflow
	if err := r.db.First(&w, "id = ?", id).Error; err != nil {
		return nil, err
	}
	w.IsActive = !w.IsActive
	err := r.db.Save(&w).Error
	return &w, err
}

func (r *Repo) UpdateWorkflowVars(id uuid.UUID, vars string) error {
	return r.db.Model(&model.Workflow{}).Where("id = ?", id).Update("variables", vars).Error
}

func (r *Repo) UpdateWorkflowCron(id uuid.UUID, cron string) error {
	return r.db.Model(&model.Workflow{}).Where("id = ?", id).Update("trigger_cron", cron).Error
}

func (r *Repo) UpdateWorkflowStopTime(id uuid.UUID, stopTime string) error {
	return r.db.Model(&model.Workflow{}).Where("id = ?", id).Update("stop_time", stopTime).Error
}

// --- Credentials ---

func (r *Repo) ListCredentials(businessID uuid.UUID) ([]model.Credential, error) {
	var items []model.Credential
	// Return credentials belonging to this business OR those marked as GLOBAL
	err := r.db.Where("business_id = ? OR is_global = true", businessID).Order("is_global desc, created_at desc").Find(&items).Error
	return items, err
}

func (r *Repo) CreateCredential(c *model.Credential) error {
	return r.db.Create(c).Error
}

func (r *Repo) DeleteCredential(id uuid.UUID) error {
	return r.db.Delete(&model.Credential{}, "id = ?", id).Error
}

func (r *Repo) GetCredential(id uuid.UUID) (*model.Credential, error) {
	var c model.Credential
	err := r.db.First(&c, "id = ?", id).Error
	return &c, err
}

// --- Executions ---

func (r *Repo) CreateExecution(e *model.Execution) error {
	return r.db.Create(e).Error
}

func (r *Repo) ListExecutions(workflowID uuid.UUID, limit int) ([]model.Execution, error) {
	var items []model.Execution
	err := r.db.Where("workflow_id = ?", workflowID).
		Order("created_at desc").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *Repo) ListExecutionsByBusiness(businessID uuid.UUID, limit int) ([]model.Execution, error) {
	var items []model.Execution
	err := r.db.
		Joins("Workflow").
		Where("\"Workflow\".business_id = ?", businessID).
		Order("executions.created_at desc").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *Repo) ListExecutionLogs(executionID uuid.UUID) ([]model.ExecutionLog, error) {
	var items []model.ExecutionLog
	err := r.db.Where("execution_id = ?", executionID).Order("created_at asc").Find(&items).Error
	return items, err
}

// --- Admin: Prompts ---

func (r *Repo) ListPromptsByBusiness(bid uuid.UUID) ([]model.ProjectPrompt, error) {
	var items []model.ProjectPrompt
	err := r.db.Where("business_id = ?", bid).Order("kind asc, version desc").Find(&items).Error
	return items, err
}

func (r *Repo) CreatePrompt(p *model.ProjectPrompt) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Deactivate current active prompt of this kind
		tx.Model(&model.ProjectPrompt{}).
			Where("business_id = ? AND kind = ? AND is_active = true", p.BusinessID, p.Kind).
			Update("is_active", false)
		
		// Find latest version
		var latestVer int
		tx.Model(&model.ProjectPrompt{}).
			Where("business_id = ? AND kind = ?", p.BusinessID, p.Kind).
			Select("MAX(version)").Scan(&latestVer)
		
		p.Version = latestVer + 1
		p.IsActive = true
		return tx.Create(p).Error
	})
}

// --- Admin: Sales ---

func (r *Repo) ListSalesByBusiness(bid uuid.UUID) ([]model.SalesAssignment, error) {
	var items []model.SalesAssignment
	err := r.db.Where("business_id = ?", bid).Order("sales_name asc").Find(&items).Error
	return items, err
}

func (r *Repo) UpsertSalesAssignment(s *model.SalesAssignment) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
		return r.db.Create(s).Error
	}
	return r.db.Save(s).Error
}

func (r *Repo) ToggleSalesActive(id uuid.UUID) error {
	return r.db.Model(&model.SalesAssignment{}).Where("id = ?", id).
		Update("is_active", gorm.Expr("NOT is_active")).Error
}

// --- Admin: Leads ---

func (r *Repo) ListLeadsExtended(bid uuid.UUID, page, limit int, search string) ([]model.Lead, int64, error) {
	var items []model.Lead
	var total int64
	
	query := r.db.Model(&model.Lead{}).Where("business_id = ?", bid)
	if search != "" {
		s := "%" + search + "%"
		query = query.Where("phone LIKE ? OR name LIKE ? OR interest LIKE ?", s, s, s)
	}
	
	query.Count(&total)
	
	err := query.Order("created_at desc").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&items).Error
		
	return items, total, err
}

func (r *Repo) ListMessagesByLead(leadID uuid.UUID) ([]model.LeadMessage, error) {
	var items []model.LeadMessage
	err := r.db.Where("lead_id = ?", leadID).Order("created_at asc").Find(&items).Error
	return items, err
}
