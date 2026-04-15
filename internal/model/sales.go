package model

import (
	"time"

	"github.com/google/uuid"
)

// SalesAssignment tracks the available salespeople for a business to enable round-robin assignment.
type SalesAssignment struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BusinessID     uuid.UUID `gorm:"type:uuid;not null;index" json:"business_id"`
	SalesName      string    `gorm:"type:varchar(128);not null" json:"sales_name"`
	SpvName        string    `gorm:"type:varchar(128)" json:"spv_name"`
	WaGroupID      string    `gorm:"type:varchar(128)" json:"wa_group_id"`
	IsActive       bool      `gorm:"not null;default:true" json:"is_active"`
	LastAssignedAt *time.Time `json:"last_assigned_at"`
	AssignCount    int       `gorm:"not null;default:0" json:"assign_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	Business       Business  `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// LeadSalesAssignment links a lead to the sales rep that was assigned to them.
type LeadSalesAssignment struct {
	LeadID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"lead_id"`
	SalesAssignmentID uuid.UUID `gorm:"type:uuid;not null;index" json:"sales_assignment_id"`
	AssignedAt        time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"assigned_at"`

	Lead              Lead            `gorm:"foreignKey:LeadID" json:"lead,omitempty"`
	SalesAssignment   SalesAssignment `gorm:"foreignKey:SalesAssignmentID" json:"sales_assignment,omitempty"`
}
