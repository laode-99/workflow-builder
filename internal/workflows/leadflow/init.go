package leadflow

import (
	"github.com/workflow-builder/core/internal/sdk"
)

// RegisterHandlers registers the leadflow multi-project engine cron handlers into the
// central worker registry. These 4 workflows form the backbone of the Leadflow system.
func RegisterHandlers() {
	sdk.RegisterWorkflow("leadflow.ingest", sdk.WorkflowDef{
		Name:        "Lead Ingestion",
		Description: "Pulls new leads from the CRM and inserts them at attempt=0",
		Handler:     handleIngest,
	})

	sdk.RegisterWorkflow("leadflow.attempt_manager", sdk.WorkflowDef{
		Name:        "Attempt Manager",
		Description: "Evaluates ALL leads against the State Machine and escalates AI calls or WA bridging",
		Handler:     handleAttemptManager,
	})

	sdk.RegisterWorkflow("leadflow.remarks_generator", sdk.WorkflowDef{
		Name:        "Remarks Generator",
		Description: "Summarizes active chatbot conversations that have been idle for >5 hours",
		Handler:     handleRemarks,
	})

	sdk.RegisterWorkflow("leadflow.wa_group_dispatch", sdk.WorkflowDef{
		Name:        "WhatsApp Group Dispatcher",
		Description: "Sends hot leads (Callback/Agent) to the sales team WA group via round-robin",
		Handler:     handleWAGroup,
	})
}
