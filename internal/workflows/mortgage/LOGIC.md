# Workflow Logic Blueprint: Mortgage AI Call

This file is the **Source of Truth** for the workflow's logical operations. It is written in human language to make it easy to modify and for AI assistants to understand exactly how the Go code is structured.

## ⚙️ Core Logic Flow

1.  **Data Ingestion (Google Sheets)**
    *   Connects to the spreadsheet using the `google_sheet_id` and `google_sheet_tab_name` provided in the variables.
    *   Fetches the first 500 rows (or up to the last populated row).

2.  **Lead Filtering**
    *   Iterates through every row found.
    *   **Skip Condition**: A row is skipped if the `Summary` column OR the `Call_Date` column already contains data (meaning it's already been processed).
    *   **Validation**: Skips rows where the `Phone Number` column is empty.

3.  **Asynchronous Call Trigger (Retell AI)**
    *   For each valid lead, it calls the **Retell v2 API**.
    *   **Dynamic Variables**: It injects the `first_name` into Retell from the `Name` column in the sheet.
    *   **Formatting**: Automatically adds the `+` prefix to the lead's phone number if it is missing.
    *   **Rate Limiting**: Waits for `delay_seconds` (defined in variables) between consecutive call triggers to prevent API throttling.

4.  **Callback Queue (Webhook)**
    *   The workflow does **not** update the sheet immediately.
    *   It registers a unique `execution_id` with Retell.
    *   When the call finishes, Retell sends a POST request to our `/webhooks/callbacks/retell` endpoint.

5.  **Result Synchronization**
    *   Upon receiving the callback, the system finds the original spreadsheet row by matching the lead's phone number.
    *   It updates the `Summary` column with the AI's call notes.
    *   It updates the `Call_Date` column with the current timestamp.

---

> [!IMPORTANT]
> **To add new logic**: Simply edit the bullet points above in your request. I will then rewrite this file and implement the corresponding Go code in `internal/workflows/mortgage/call_leads.go`.
