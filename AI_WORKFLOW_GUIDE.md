# AI Developer Guide: Code-as-Workflow Platform

**⚠️ IMPORTANT FOR AI AGENTS:** Read this document carefully before generating or modifying any workflows for this system.

## 1. System Context
This project uses a **Code-as-Workflow** paradigm. We do not use visual nodes (like n8n or Zapier) or JSON DSL definitions. Instead, every workflow is written as pure, strongly-typed **Go code** utilizing our custom Platform SDK. 

The UI (Next.js Dashboard) is strictly for operational management (enabling workflows, setting cron triggers, filling credential variables). **Business logic lives here in the Go codebase.**

## 2. Folder Structure
When the user asks you to create or update a workflow (e.g., "in project A -> workflow B -> do this..."), follow this strict convention:

```text
internal/workflows/
  ├── [project_name_snake_case]/
  │     ├── [workflow_name_snake_case].go
```

**Example:** If the user says "In project Mortgage, create an Outbound Call workflow", you will create: `internal/workflows/mortgage/outbound_call.go`

## 3. The SDK Paradigm
When writing a workflow, you must use our `sdk` and `integrations` packages. 

**Rule 1: Never hardcode credentials.** 
All credentials (API Keys, OAuth tokens) are securely handled by the system. You just instantiate the client using `exec.BusinessID()`. The system will automatically fetch the right keys.

**Rule 2: Expose UI Variables.**
If a workflow needs specific IDs (like a Target Google Sheet ID or Retell Agent ID), declare them in the `RequiredVars` slice. The UI will automatically generate form fields for the human user to fill out.

## 4. Standard Workflow Template
Copy and adapt this template when generating a new workflow based on the user's human-language description:

```go
package projectname // e.g., package mortgage

import (
    "context"
    "time"

    "github.com/workflow-builder/core/internal/integrations/gsheets"
    "github.com/workflow-builder/core/internal/integrations/retell" // import integrations as requested
    "github.com/workflow-builder/core/internal/sdk"
)

func init() {
    sdk.RegisterWorkflow("UniqueWorkflowSignatureLike_ProjectAB_TaskC", sdk.WorkflowDef{
        Name:        "Human Readable Workflow Name",
        Description: "Description of what this does",
        RequiredVars: []string{
            "google_sheet_id", 
            "custom_variable_user_needs_to_input",
        },
        Handler: TriggerLogic,
    })
    
    // (Optional) Register Webhook if the workflow relies on an external callback
    sdk.RegisterWebhook("CallbackSignature", sdk.WebhookDef{
        Path: "/callbacks/unique-path",
        Func: WebhookLogic,
    })
}

// TriggerLogic executes when the cron runs or the user clicks "Play"
func TriggerLogic(ctx context.Context, exec sdk.Execution) error {
    log := exec.Logger()
    
    // 1. Fetch Variables set by the User in the Dashboard
    sheetID := exec.GetVar("google_sheet_id")
    
    // 2. Instantiate integration SDKs automatically reusing credentials
    // The integration automatically uses the Tenant/Business vault
    sheets := gsheets.NewClient(ctx, exec.BusinessID())
    
    // 3. Main Business Logic
    // ...
    // ALWAYS check if the user clicked "Force Stop" in the UI during a loop
    for _, item := range dataList {
        if exec.IsStopped() {
            log.Info("Forcefully stopped by user")
            return nil
        }
        // ... do something with item
    }
    
    return nil
}

// WebhookLogic (Optional) executes when an external service calls back
func WebhookLogic(ctx context.Context, query map[string]string, payload []byte) error {
     // implementation... 
     // Use sdk.ReconstructExecution(ctx, "exec_id") to resume state.
     return nil
}
```

## 5. Instructions for AI Translation
When a user provides an n8n JSON snippet or explains a workflow in human language:
1. Identify the **trigger** (is it a cron schedule? manual? webhook in?).
2. Identify the **integrations needed** (Google Sheets, Gmail, Retell, HTTP).
3. Identify the **logic flow** (If statements, For loops, Delays).
4. Translate it directly into the Go struct format above. 
5. Provide a summary explaining how the user should fill out the variables in their UI Dashboard once you've deployed the code.
