package repo

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/model"
	"gorm.io/gorm"
)

type PromptRepo struct {
	db *gorm.DB
}

func NewPromptRepo(db *gorm.DB) *PromptRepo {
	return &PromptRepo{db: db}
}

// GetActive returns the currently active prompt for a given kind and business.
func (r *PromptRepo) GetActive(ctx context.Context, businessID uuid.UUID, kind string) (*model.ProjectPrompt, error) {
	var prompt model.ProjectPrompt
	err := r.db.WithContext(ctx).
		Where("business_id = ? AND kind = ? AND is_active = true", businessID, kind).
		Order("version DESC").
		First(&prompt).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get active prompt: %w", err)
	}
	
	return &prompt, nil
}
