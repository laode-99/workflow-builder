package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"gorm.io/gorm"
)

type SalesRepo struct {
	db *gorm.DB
}

func NewSalesRepo(db *gorm.DB) *SalesRepo {
	return &SalesRepo{db: db}
}

// GetNextForAssignment selects the next active salesperson for a project 
// using a round-robin strategy (least recently assigned first).
func (r *SalesRepo) GetNextForAssignment(ctx context.Context, businessID uuid.UUID) (*model.SalesAssignment, error) {
	var assignment model.SalesAssignment
	err := r.db.WithContext(ctx).
		Where("business_id = ? AND is_active = true", businessID).
		Order("last_assigned_at ASC NULLS FIRST, assign_count ASC").
		First(&assignment).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get next sales assignment: %w", err)
	}
	
	return &assignment, nil
}

// RecordAssignment increments the count and updates the timestamp for a salesperson.
func (r *SalesRepo) RecordAssignment(ctx context.Context, id uuid.UUID, leadID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Update SalesAssignment
		res := tx.Model(&model.SalesAssignment{}).
			Where("id = ?", id).
			Updates(map[string]any{
				"last_assigned_at": time.Now(),
				"assign_count":     gorm.Expr("assign_count + 1"),
			})
		if res.Error != nil {
			return res.Error
		}

		// 2. Clear previous assignment for this lead if any
		if err := tx.Where("lead_id = ?", leadID).Delete(&model.LeadSalesAssignment{}).Error; err != nil {
			return err
		}

		// 3. Insert new LeadSalesAssignment
		link := model.LeadSalesAssignment{
			LeadID:            leadID,
			SalesAssignmentID: id,
			AssignedAt:        time.Now(),
		}
		return tx.Create(&link).Error
	})
}

// ListActiveAssignments returns all active sales reps for a project.
func (r *SalesRepo) ListActiveAssignments(ctx context.Context, businessID uuid.UUID) ([]model.SalesAssignment, error) {
	var list []model.SalesAssignment
	err := r.db.WithContext(ctx).
		Where("business_id = ? AND is_active = true", businessID).
		Find(&list).Error
	return list, err
}
