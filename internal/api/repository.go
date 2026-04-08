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
