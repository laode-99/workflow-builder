package api

import (
	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// --- Businesses ---

func (r *Repo) CreateBusiness(b *model.Business) error {
	return r.db.Create(b).Error
}

func (r *Repo) ListBusinesses() ([]model.Business, error) {
	items := []model.Business{}
	err := r.db.Order("name asc").Find(&items).Error
	return items, err
}

func (r *Repo) GetBusiness(id uuid.UUID) (*model.Business, error) {
	var b model.Business
	err := r.db.First(&b, "id = ?", id).Error
	return &b, err
}

func (r *Repo) GetBusinessBySlug(slug string) (*model.Business, error) {
	var b model.Business
	err := r.db.First(&b, "slug = ?", slug).Error
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repo) DeleteBusiness(id uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete executions associated with workflows of this business
		if err := tx.Exec("DELETE FROM executions WHERE workflow_id IN (SELECT id FROM workflows WHERE business_id = ?)", id).Error; err != nil {
			return err
		}
		// Delete credentials
		if err := tx.Where("business_id = ?", id).Delete(&model.Credential{}).Error; err != nil {
			return err
		}
		// Delete workflows
		if err := tx.Where("business_id = ?", id).Delete(&model.Workflow{}).Error; err != nil {
			return err
		}
		// Delete audit logs
		if err := tx.Where("business_id = ?", id).Delete(&model.AuditLog{}).Error; err != nil {
			return err
		}
		// Delete business
		if err := tx.Delete(&model.Business{}, "id = ?", id).Error; err != nil {
			return err
		}
		return nil
	})
}

// --- Credentials ---

func (r *Repo) CreateCredential(c *model.Credential) error {
	return r.db.Create(c).Error
}

func (r *Repo) ListCredentials(businessID uuid.UUID) ([]model.Credential, error) {
	items := []model.Credential{}
	err := r.db.Where("business_id = ? OR is_global = true", businessID).Order("label asc").Find(&items).Error
	return items, err
}

func (r *Repo) GetCredential(id uuid.UUID) (*model.Credential, error) {
	var c model.Credential
	err := r.db.First(&c, "id = ?", id).Error
	return &c, err
}

func (r *Repo) DeleteCredential(id uuid.UUID) error {
	return r.db.Delete(&model.Credential{}, "id = ?", id).Error
}

// --- Workflows ---

func (r *Repo) CreateWorkflow(w *model.Workflow) error {
	return r.db.Create(w).Error
}

func (r *Repo) ListWorkflows(businessID uuid.UUID) ([]model.Workflow, error) {
	items := []model.Workflow{}
	err := r.db.Where("business_id = ?", businessID).Order("alias asc").Find(&items).Error
	return items, err
}

func (r *Repo) GetWorkflow(id uuid.UUID) (*model.Workflow, error) {
	var w model.Workflow
	err := r.db.First(&w, "id = ?", id).Error
	return &w, err
}


func (r *Repo) ToggleWorkflow(id uuid.UUID) (*model.Workflow, error) {
	var w model.Workflow
	if err := r.db.First(&w, "id = ?", id).Error; err != nil {
		return nil, err
	}
	w.IsActive = !w.IsActive
	if err := r.db.Save(&w).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *Repo) UpdateWorkflowCron(id uuid.UUID, cron string) error {
	return r.db.Model(&model.Workflow{}).Where("id = ?", id).Update("trigger_cron", cron).Error
}

func (r *Repo) UpdateWorkflowStopTime(id uuid.UUID, stopTime string) error {
	return r.db.Model(&model.Workflow{}).Where("id = ?", id).Update("stop_time", stopTime).Error
}

func (r *Repo) UpdateWorkflowVars(id uuid.UUID, vars string) error {
	return r.db.Model(&model.Workflow{}).Where("id = ?", id).Update("variables", vars).Error
}

func (r *Repo) UpdateWorkflow(w *model.Workflow) error {
	return r.db.Save(w).Error
}

func (r *Repo) DeleteWorkflow(id uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete executions
		if err := tx.Where("workflow_id = ?", id).Delete(&model.Execution{}).Error; err != nil {
			return err
		}
		// Delete workflow
		if err := tx.Delete(&model.Workflow{}, "id = ?", id).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *Repo) ListAllActiveWorkflows() ([]model.Workflow, error) {
	var items []model.Workflow
	err := r.db.Where("is_active = ?", true).Find(&items).Error
	return items, err
}

// --- Executions ---

func (r *Repo) CreateExecution(e *model.Execution) error {
	return r.db.Create(e).Error
}

func (r *Repo) ListExecutions(workflowID uuid.UUID, limit int) ([]model.Execution, error) {
	items := []model.Execution{}
	err := r.db.Where("workflow_id = ?", workflowID).Order("created_at desc").Limit(limit).Find(&items).Error
	return items, err
}

func (r *Repo) ListExecutionsByBusiness(businessID uuid.UUID, limit int) ([]model.Execution, error) {
	items := []model.Execution{}
	err := r.db.
		Table("executions").
		Joins("JOIN workflows ON workflows.id = executions.workflow_id").
		Where("workflows.business_id = ?", businessID).
		Order("executions.created_at desc").
		Limit(limit).
		Find(&items).Error
	return items, err
}

func (r *Repo) GetExecution(id uuid.UUID) (*model.Execution, error) {
	var e model.Execution
	err := r.db.First(&e, "id = ?", id).Error
	return &e, err
}

func (r *Repo) UpdateExecution(e *model.Execution) error {
	return r.db.Save(e).Error
}

func (r *Repo) CreateExecutionLog(l *model.ExecutionLog) error {
	return r.db.Create(l).Error
}

func (r *Repo) ListExecutionLogs(executionID uuid.UUID) ([]model.ExecutionLog, error) {
	items := []model.ExecutionLog{}
	err := r.db.Where("execution_id = ?", executionID).Order("created_at asc").Find(&items).Error
	return items, err
}

// --- Users ---

func (r *Repo) GetUserByEmail(email string) (*model.User, error) {
	var u model.User
	err := r.db.First(&u, "email = ?", email).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) ListUsers() ([]model.User, error) {
	items := []model.User{}
	err := r.db.Order("name asc").Find(&items).Error
	return items, err
}

func (r *Repo) DeleteUser(id uuid.UUID) error {
	return r.db.Delete(&model.User{}, "id = ?", id).Error
}

func (r *Repo) CreateUser(u *model.User) error {
	return r.db.Create(u).Error
}

// --- Audit Logs ---

func (r *Repo) CreateAuditLog(al *model.AuditLog) error {
	return r.db.Create(al).Error
}

func (r *Repo) ListAuditLogs(businessID uuid.UUID, limit int) ([]model.AuditLog, error) {
	items := []model.AuditLog{}
	err := r.db.Where("business_id = ?", businessID).
		Order("created_at desc").
		Limit(limit).
		Find(&items).Error
	return items, err
}

// --- Prompts ---

func (r *Repo) ListPromptsByBusiness(businessID uuid.UUID) ([]model.ProjectPrompt, error) {
	items := []model.ProjectPrompt{}
	err := r.db.Where("business_id = ?", businessID).Order("kind asc, version desc").Find(&items).Error
	return items, err
}

func (r *Repo) CreatePrompt(p *model.ProjectPrompt) error {
	return r.db.Create(p).Error
}

// --- Sales ---

func (r *Repo) ListSalesByBusiness(businessID uuid.UUID) ([]model.SalesAssignment, error) {
	items := []model.SalesAssignment{}
	err := r.db.Where("business_id = ?", businessID).Order("sales_name asc").Find(&items).Error
	return items, err
}

func (r *Repo) UpsertSalesAssignment(s *model.SalesAssignment) error {
	return r.db.Save(s).Error
}

func (r *Repo) ToggleSalesActive(id uuid.UUID) error {
	return r.db.Model(&model.SalesAssignment{}).Where("id = ?", id).
		Update("is_active", gorm.Expr("NOT is_active")).Error
}

// --- Leads & Messaging ---

func (r *Repo) ListLeadsExtended(businessID uuid.UUID, page, limit int, search string) ([]model.Lead, int64, error) {
	var items []model.Lead
	var total int64
	
	query := r.db.Model(&model.Lead{}).Where("business_id = ?", businessID)
	if search != "" {
		s := "%" + search + "%"
		query = query.Where("name ILIKE ? OR phone ILIKE ? OR external_id ILIKE ?", s, s, s)
	}
	
	query.Count(&total)
	err := query.Order("created_at desc").Offset((page - 1) * limit).Limit(limit).Find(&items).Error
	return items, total, err
}

func (r *Repo) ListMessagesByLead(leadID uuid.UUID) ([]model.LeadMessage, error) {
	items := []model.LeadMessage{}
	err := r.db.Where("lead_id = ?", leadID).Order("created_at asc").Find(&items).Error
	return items, err
}
