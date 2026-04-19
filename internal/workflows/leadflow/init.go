package leadflow

import (
	"github.com/workflow-builder/core/internal/sdk"
)

// init registers the leadflow multi-project engine cron handlers into the
// central worker registry. These 4 workflows form the backbone of the Leadflow system.
func init() {
	sdk.RegisterWorkflow("leadflow.ingest", sdk.WorkflowDef{
		Name:        "Leadflow: 1. Lead Ingestion",
		Description: "Pulls new leads from the CRM (LeadSquared) and initializes them in the engine.",
		Category:    "ai_engine",
		Steps: []sdk.Step{
			{ID: "fetch", Label: "Fetch from CRM", Icon: "Download", Description: "Pulls latest opportunities based on configured Activity Events."},
			{ID: "dedupe", Label: "Deduplicate", Icon: "CheckCircle", Description: "Ensures lead uniqueness and avoids double-calling."},
			{ID: "ingest", Label: "Ingest", Icon: "Database", Description: "Creates the local Lead record with Attempt 0 state."},
		},
		Handler:     handleIngest,
	})

	sdk.RegisterWorkflow("leadflow.attempt_manager", sdk.WorkflowDef{
		Name:        "Leadflow: 2. Attempt Escalation Manager",
		Description: "Evaluates all active leads against the State Machine and triggers AI calls or WA bridging.",
		Category:    "ai_engine",
		Steps: []sdk.Step{
			{ID: "scan", Label: "Scan Leads", Icon: "Search", Description: "Identifies leads ready for their next attempt (Cooldown check)."},
			{ID: "transition", Label: "State Machine", Icon: "Cpu", Description: "Runs logic to decide between Call, WhatsApp, or Wait."},
			{ID: "dispatch", Label: "Dispatch", Icon: "Zap", Description: "Enqueues tasks for Retell AI or WhatsApp API."},
		},
		Handler:     handleAttemptManager,
	})

	sdk.RegisterWorkflow("leadflow.remarks_generator", sdk.WorkflowDef{
		Name:        "Leadflow: 3. AI Remarks Generator",
		Description: "Summarizes active chatbot conversations that have been idle for >5 hours.",
		Category:    "ai_engine",
		Steps: []sdk.Step{
			{ID: "monitor", Label: "Monitor Chat", Icon: "MessageCircle", Description: "Checks for finished or idle WhatsApp conversations."},
			{ID: "summarize", Label: "AI Summary", Icon: "FileText", Description: "Uses LLM to condense the chat history into a CRM-ready note."},
			{ID: "sync", Label: "Sync to CRM", Icon: "RefreshCw", Description: "Writes the summary back to the LeadSquared opportunity."},
		},
		Handler:     handleRemarks,
	})

	sdk.RegisterWorkflow("leadflow.wa_group_dispatch", sdk.WorkflowDef{
		Name:        "Leadflow: 4. WhatsApp Sales Dispatcher",
		Description: "Sends hot leads (Callback/Agent) to the sales team WA groups via round-robin.",
		Category:    "ai_engine",
		Steps: []sdk.Step{
			{ID: "audit", Label: "Audit Check", Icon: "Shield", Description: "Ensures no double-dispatch for the same intent."},
			{ID: "round_robin", Label: "Assignment", Icon: "Users", Description: "Finds the next available sales group for this project."},
			{ID: "notify", Label: "WA Notify", Icon: "Send", Description: "Formatted alert sent to the Gupshup/WA group."},
		},
		Handler:     handleWAGroup,
	})
}
