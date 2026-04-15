package leadsquared

// CRM field-name constants mirroring the Anandaya reference implementation.
// A new project reuses these defaults unless its FlowConfig.crm.field_mapping
// provides overrides. (Per-project overrides are a Phase 2 concern — MVP
// assumes all new projects share Anandaya's LeadSquared custom field numbers.)
const (
	FieldContactStatus  = "mx_Custom_25" // Connected | Not Connected | Not Valid
	FieldCallDate       = "mx_Custom_26"
	FieldVisitScheduled = "mx_Custom_28" // "Visit Scheduled" | ""
	FieldVisitDate      = "mx_Custom_29" // svs_date string
	FieldContactResult  = "mx_Custom_54" // Interest Project | No Pick Up | Spam | Agent | Callback | ...
	FieldIsInterested   = "mx_Custom_56" // Yes | No
	FieldCallDate2      = "mx_Custom_57"
	FieldSummary        = "mx_Custom_75" // "Call Notes: ... | Chat Notes: ..."
	FieldContactResult2 = "mx_Custom_81" // duplicate of mx_Custom_54
)

// PathA is the "responded / substantive conversation" CRM update path.
// Matches the Anandaya MAIN SWITCH Path A branch.
type PathA struct {
	ContactStatus   string // Connected
	ContactResult   string // Interest Project | Callback | ...
	IsInterested    string // Yes | No
	VisitScheduled  string // "Visit Scheduled" | ""
	VisitDate       string // svs_date string (pre-formatted)
	CallDate        string // formatted timestamp
	Summary         string // "Call Notes: ... | Chat Notes: ..."
	OpportunityID   string
}

// ToFieldUpdates renders a PathA struct as the field-update list the
// LeadSquared UpdateOpportunity call expects.
func (p PathA) ToFieldUpdates() []FieldUpdate {
	return []FieldUpdate{
		{SchemaName: FieldContactStatus, Value: p.ContactStatus},
		{SchemaName: FieldCallDate, Value: p.CallDate},
		{SchemaName: FieldContactResult, Value: p.ContactResult},
		{SchemaName: FieldContactResult2, Value: p.ContactResult},
		{SchemaName: FieldIsInterested, Value: p.IsInterested},
		{SchemaName: FieldCallDate2, Value: p.CallDate},
		{SchemaName: FieldSummary, Value: p.Summary},
		{SchemaName: FieldVisitScheduled, Value: p.VisitScheduled},
		{SchemaName: FieldVisitDate, Value: p.VisitDate},
	}
}

// PathB is the "no-response / cron-driven max-out" CRM update path.
type PathB struct {
	ContactStatus string // Not Connected | Not Valid
	ContactResult string // No Pick Up | (empty)
	CallDate      string
	Summary       string
	OpportunityID string
}

// ToFieldUpdates renders a PathB struct as field updates.
func (p PathB) ToFieldUpdates() []FieldUpdate {
	return []FieldUpdate{
		{SchemaName: FieldContactStatus, Value: p.ContactStatus},
		{SchemaName: FieldCallDate, Value: p.CallDate},
		{SchemaName: FieldContactResult, Value: p.ContactResult},
		{SchemaName: FieldContactResult2, Value: p.ContactResult},
		{SchemaName: FieldCallDate2, Value: p.CallDate},
		{SchemaName: FieldSummary, Value: p.Summary},
	}
}
