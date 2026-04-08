const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `API ${res.status}`);
  }
  return res.json();
}

// --- Types ---

export interface Business {
  id: string;
  name: string;
  slug: string;
  created_at: string;
}

export interface Step {
  id: string;
  label: string;
  icon: string;
  description: string;
}

export interface RegistryItem {
  signature: string;
  name: string;
  description: string;
  params: WorkflowParam[];
  steps: Step[];
}

export interface Credential {
  id: string;
  business_id: string;
  label: string;
  integration: string;
  is_global: boolean;
  created_at: string;
}

export interface WorkflowParam {
  key: string;
  type: "string" | "credential";
  description?: string;
  integration?: string;
}

export interface Workflow {
  id: string;
  business_id: string;
  signature: string;
  alias: string;
  is_active: boolean;
  trigger_cron?: string;
  stop_time?: string;
  variables: string;
  sdk_name?: string;
  sdk_description?: string;
  params?: WorkflowParam[];
  steps?: Step[];
  created_at: string;
}

export interface Execution {
  id: string;
  workflow_id: string;
  status: string;
  error_msg: string;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
  workflow?: Workflow;
}

export interface ExecutionLog {
  id: string;
  execution_id: string;
  level: string;
  message: string;
  created_at: string;
}

// --- API functions ---

export const api = {
  // Registry
  getRegistry: () => request<RegistryItem[]>("/api/registry"),

  // Businesses
  listBusinesses: () => request<Business[]>("/api/businesses"),
  createBusiness: (name: string) =>
    request<Business>("/api/businesses", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  deleteBusiness: (id: string) =>
    request<{ ok: boolean }>(`/api/businesses/${id}`, { method: "DELETE" }),

  // Workflows
  listWorkflows: (bid: string) =>
    request<Workflow[]>(`/api/businesses/${bid}/workflows`),
  createWorkflow: (bid: string, data: { signature: string; alias: string; trigger_cron: string }) =>
    request<Workflow>(`/api/businesses/${bid}/workflows`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  toggleWorkflow: (id: string) =>
    request<Workflow>(`/api/workflows/${id}/toggle`, { method: "PATCH" }),
  updateWorkflowVars: (id: string, variables: string) =>
    request<{ ok: boolean }>(`/api/workflows/${id}/variables`, {
      method: "PATCH",
      body: JSON.stringify({ variables }),
    }),
  updateWorkflowCron: (id: string, trigger_cron: string) =>
    request<{ ok: boolean }>(`/api/workflows/${id}/cron`, {
      method: "PATCH",
      body: JSON.stringify({ trigger_cron }),
    }),
  updateWorkflowStopTime: (id: string, stop_time: string) =>
    request<{ ok: boolean }>(`/api/workflows/${id}/stop-time`, {
      method: "PATCH",
      body: JSON.stringify({ stop_time }),
    }),
  deleteWorkflow: (id: string) =>
    request<{ ok: boolean }>(`/api/workflows/${id}`, { method: "DELETE" }),

  // Credentials
  listCredentials: (bid: string) =>
    request<Credential[]>(`/api/businesses/${bid}/credentials`),
  createCredential: (bid: string, data: { label: string; integration: string; data: string; is_global: boolean }) =>
    request<Credential>(`/api/businesses/${bid}/credentials`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  deleteCredential: (id: string) =>
    request<{ ok: boolean }>(`/api/credentials/${id}`, { method: "DELETE" }),
  verifyCredential: (id: string) =>
    request<{ ok: boolean }>(`/api/credentials/${id}/verify`, { method: "POST" }),
  previewCredentialData: (id: string, sheet_id: string, tab_name: string) =>
    request<any[]>(`/api/credentials/${id}/preview`, { 
      method: "POST",
      body: JSON.stringify({ sheet_id, tab_name })
    }),

  // Executions
  listExecutions: (bid: string) =>
    request<Execution[]>(`/api/businesses/${bid}/executions`),

  // Trigger / Stop
  triggerWorkflow: (id: string) =>
    request<Execution>(`/api/workflows/${id}/trigger`, { method: "POST" }),
  stopWorkflow: (id: string) =>
    request<{ ok: boolean }>(`/api/workflows/${id}/stop`, { method: "POST" }),

  // Logs
  getExecutionLogs: (id: string) =>
    request<any[]>(`/api/executions/${id}/logs`),

  // Logic
  getWorkflowLogic: (id: string) =>
    request<{ content: string }>(`/api/workflows/${id}/logic`),
};
