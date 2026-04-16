package leadsquared

import (
	"strings"
	"time"

	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/statemachine"
)

// CRM field constants — verbatim from the Anandaya reference implementation
// (docs/SCALABLE_FLOW_LOGIC.md + ANANDAYA_SYSTEM_DOCUMENTATION.md).
//
// These MUST stay aligned with whatever LeadSquared tenant is in use. For
// the first new project we assume the same custom-field numbering as
// Anandaya; if a future project's LeadSquared tenant uses different numbers
// the override lives in FlowConfig.crm.field_mapping (Phase 2).
const (
	FieldContactStatus  = "mx_Custom_25" // Connected | Not Connected | Not Valid
	FieldCallDate       = "mx_Custom_26" // formatted timestamp
	FieldVisitScheduled = "mx_Custom_28" // "Visit Scheduled" | ""
	FieldSvsDate        = "mx_Custom_29" // M/D/YYYY H:mm:ss AM/PM
	FieldContactResult  = "mx_Custom_54" // Interest Project | No Pick Up | Spam | Agent | Callback | ...
	FieldIsInterested   = "mx_Custom_56" // Yes | No
	FieldCallDate2      = "mx_Custom_57" // duplicate of call_date (Anandaya quirk)
	FieldSummary        = "mx_Custom_75" // "Call Notes: ... | Chat Notes: ..."
	FieldContactResult2 = "mx_Custom_81" // duplicate of contact_result (Anandaya quirk)
)

// Contact status values (mx_Custom_25).
const (
	StatusConnected    = "Connected"
	StatusNotConnected = "Not Connected"
	StatusNotValid     = "Not Valid"
)

// Contact result values (mx_Custom_54 + 81).
const (
	ResultInterestProject = "Interest Project"
	ResultNoPickUp        = "No Pick Up"
	ResultSpam            = "Spam"
	ResultAgent           = "Agent"
	ResultCallback        = "Callback"
	ResultUnqualified     = "Unqualified"
	ResultDoubleData      = "Double Data"
	ResultNotValid        = "Not Valid"
)

// FieldUpdate already defined in client.go.

// BuildPathAUpdates produces the LeadSquared field updates for a Path A
// (responded) CRM push. Path A fires when the lead either replied via WA,
// completed a substantive phone conversation, or was classified by the
// chatbot intent classifier.
//
// The precedence order matches Anandaya's MAIN SWITCH in WF1-F:
//  1. Chatbot intent (interest2) overrides everything if set
//  2. Retell call interest (interest) is the next strongest signal
//  3. Customer type is the final fallback
//
// Rationale: intent2 is the latest write in the lead's timeline. A lead
// might say "Tertarik Site Visit" to the AI caller but later tell the
// chatbot "tidak tertarik" — the chatbot wins because it's more recent.
func BuildPathAUpdates(lead *model.Lead) []FieldUpdate {
	contactStatus := StatusConnected
	contactResult := ResultInterestProject
	isInterested := "Yes"
	visitScheduled := ""
	svsDate := ""

	// 1. Chatbot intent takes precedence (chronologically last).
	switch strings.TrimSpace(lead.Interest2) {
	case statemachine.IntentTidakTertarik:
		contactResult = ResultSpam
		isInterested = "No"
	case statemachine.IntentAgent:
		contactResult = ResultAgent
		isInterested = "No"
	case statemachine.IntentCallback:
		contactResult = ResultCallback
		isInterested = "Yes"
	default:
		// 2. Fall back to Retell interest.
		lower := strings.ToLower(lead.Interest)
		switch {
		case strings.Contains(lower, "tertarik site visit"):
			contactResult = ResultInterestProject
			isInterested = "Yes"
			visitScheduled = "Visit Scheduled"
			if lead.SvsDate != nil {
				svsDate = statemachine.FormatSvsDate(*lead.SvsDate)
			}
		case strings.Contains(lower, "tertarik di informasikan"),
			strings.Contains(lower, "tertarik untuk dihubungi"):
			contactResult = ResultInterestProject
			isInterested = "Yes"
		case strings.Contains(lower, "tidak mau"),
			strings.Contains(lower, "tidak tertarik"):
			contactResult = ResultSpam
			isInterested = "No"
		default:
			// 3. Final fallback: customer_type from Retell.
			switch lead.CustomerType {
			case "Callback":
				contactResult = ResultCallback
				isInterested = "Yes"
			case "Agent":
				contactResult = ResultAgent
				isInterested = "No"
			case "Unqualified":
				contactResult = ResultUnqualified
				isInterested = "No"
			case "Spam":
				contactResult = ResultSpam
				isInterested = "No"
			case "Double Number":
				contactResult = ResultDoubleData
				isInterested = "No"
			case "Interest":
				contactResult = ResultCallback
				isInterested = "Yes"
			default:
				// Unknown — conservative: Callback for manual review.
				contactResult = ResultCallback
				isInterested = "Yes"
			}
		}
	}

	return []FieldUpdate{
		{SchemaName: FieldContactStatus, Value: contactStatus},
		{SchemaName: FieldCallDate, Value: formatCallDate(lead.CallDate)},
		{SchemaName: FieldVisitScheduled, Value: visitScheduled},
		{SchemaName: FieldSvsDate, Value: svsDate},
		{SchemaName: FieldContactResult, Value: contactResult},
		{SchemaName: FieldIsInterested, Value: isInterested},
		{SchemaName: FieldCallDate2, Value: formatCallDate(lead.CallDate)},
		{SchemaName: FieldSummary, Value: lead.Summary},
		{SchemaName: FieldContactResult2, Value: contactResult},
	}
}

// BuildPathBUpdates produces the LeadSquared field updates for a Path B
// (no response) CRM push. Path B fires when a call failed to connect or
// the lead has been maxed out with no WA reply.
func BuildPathBUpdates(lead *model.Lead) []FieldUpdate {
	contactStatus := StatusNotConnected
	contactResult := ResultNoPickUp

	switch {
	case lead.TerminalInvalid ||
		lead.DisconnectedReason == statemachine.ReasonInvalidDestination:
		contactStatus = StatusNotValid
		contactResult = ResultNotValid

	case lead.DisconnectedReason == statemachine.ReasonVoicemailReached ||
		lead.DisconnectedReason == statemachine.ReasonIVRReached ||
		lead.CustomerType == "Voicemail":
		contactStatus = StatusNotConnected
		contactResult = ResultNoPickUp
	}

	return []FieldUpdate{
		{SchemaName: FieldContactStatus, Value: contactStatus},
		{SchemaName: FieldCallDate, Value: formatCallDate(lead.CallDate)},
		{SchemaName: FieldContactResult, Value: contactResult},
		{SchemaName: FieldCallDate2, Value: formatCallDate(lead.CallDate)},
		{SchemaName: FieldSummary, Value: lead.Summary},
		{SchemaName: FieldContactResult2, Value: contactResult},
	}
}

// BuildUpdatesForPath dispatches to the correct builder based on the
// CRM intent's path label ("A" or "B"). Used by the CRM sync outbox poller.
func BuildUpdatesForPath(lead *model.Lead, path string) []FieldUpdate {
	if path == statemachine.CRMPathResponded {
		return BuildPathAUpdates(lead)
	}
	return BuildPathBUpdates(lead)
}

// formatCallDate renders a call date in the LeadSquared-expected format.
// Returns empty string if the pointer is nil.
func formatCallDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	// Anandaya stores as "dd-MMM-yy HH:mm" (e.g., "16-Apr-26 14:30") in sheets
	// but LeadSquared accepts ISO-ish formats. We use the format that WF1-F
	// writes in the reference: "2006-01-02 15:04:05".
	return t.Format("2006-01-02 15:04:05")
}

// MapLeadToUpdates is retained for backward compatibility with existing
// callers that don't yet know about Path A/B. It delegates to Path A.
// New callers should call BuildUpdatesForPath directly.
func MapLeadToUpdates(lead *model.Lead, _ *model.SalesAssignment) []FieldUpdate {
	return BuildPathAUpdates(lead)
}
