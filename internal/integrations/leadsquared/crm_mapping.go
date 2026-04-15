package leadsquared

import (
	"fmt"
	"time"

	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/statemachine"
)

// Anandaya Field Mapping Constants
// Verbatim from docs/SCALABLE_FLOW_LOGIC.md and ANANDAYA_SYSTEM_DOCUMENTATION.md
const (
	FieldDisconnectedReason = "mx_Custom_25"
	FieldRetellInterest     = "mx_Custom_26"
	FieldVisitScheduled     = "mx_Custom_28" // "Visit Scheduled"
	FieldSvsDate            = "mx_Custom_29" // M/D/YYYY H:mm:ss AM/PM
	FieldCrmPushTimestamp   = "mx_Custom_54"
	FieldAttempt            = "mx_Custom_56"
	FieldChatbotInterest    = "mx_Custom_57" // redundant but required
	FieldSummary            = "mx_Custom_75"
	FieldSalesName          = "mx_Custom_81"
	FieldSpvName            = "mx_Custom_82"
)

// MapLeadToUpdates converts a lead model to a set of LeadSquared field updates
// based on the Anandaya project mapping.
func MapLeadToUpdates(l *model.Lead, sales *model.SalesAssignment) []FieldUpdate {
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	
	updates := []FieldUpdate{
		{SchemaName: FieldAttempt, Value: fmt.Sprintf("%d", l.Attempt)},
		{SchemaName: FieldCrmPushTimestamp, Value: nowStr},
	}

	if l.DisconnectedReason != "" {
		updates = append(updates, FieldUpdate{SchemaName: FieldDisconnectedReason, Value: l.DisconnectedReason})
	}
	
	if l.Interest != "" {
		updates = append(updates, FieldUpdate{SchemaName: FieldRetellInterest, Value: l.Interest})
	}
	
	if l.Interest2 != "" {
		updates = append(updates, FieldUpdate{SchemaName: FieldChatbotInterest, Value: l.Interest2})
	}

	if l.Summary != "" {
		updates = append(updates, FieldUpdate{SchemaName: FieldSummary, Value: l.Summary})
	}

	// SVS Date formatting
	if l.SvsDate != nil {
		updates = append(updates, FieldUpdate{SchemaName: FieldVisitScheduled, Value: "Visit Scheduled"})
		updates = append(updates, FieldUpdate{SchemaName: FieldSvsDate, Value: statemachine.FormatSvsDate(*l.SvsDate)})
	}

	// Sales assignment
	if sales != nil {
		updates = append(updates, FieldUpdate{SchemaName: FieldSalesName, Value: sales.SalesName})
		if sales.SpvName != "" {
			updates = append(updates, FieldUpdate{SchemaName: FieldSpvName, Value: sales.SpvName})
		}
	}

	return updates
}
