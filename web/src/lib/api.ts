const API = (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080").replace(/\/$/, "");

// Admin API key used as the X-API-Key header on every protected request.
// Must match the ADMIN_API_KEY env var on the Go API side (middleware_auth.go).
// Dev default is "admin-secret-dev" — override via NEXT_PUBLIC_ADMIN_API_KEY
// in web/.env.local for staging/production.
const ADMIN_API_KEY = process.env.NEXT_PUBLIC_ADMIN_API_KEY || "admin-secret-dev";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = {
    "Content-Type": "application/json",
    "X-API-Key": ADMIN_API_KEY,
    ...(init?.headers || {}),
  };
  const res = await fetch(`${API}${path}`, {
    ...init,
    headers,
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

// --- Leadflow Types ---

export interface Lead {
  id: string;
  business_id: string;
  external_id: string;
  phone: string;
  name: string;
  attempt: number;
  interest: string;
  interest2: string;
  svs_date: string | null;
  summary: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface ProjectPrompt {
  id: string;
  business_id: string;
  kind: string;
  content: string;
  version: number;
  is_active: boolean;
  created_at: string;
}

export interface SalesAssignment {
  id: string;
  business_id: string;
  sales_name: string;
  spv_name: string;
  is_active: boolean;
  created_at: string;
}

export interface ChatMessage {
  id: string;
  lead_id: string;
  role: "user" | "assistant" | "system";
  content: string;
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

  // Leadflow Admin
  listLeads: (bid: string, page = 1, limit = 50, search = "") =>
    request<{ items: Lead[]; total: number; page: number; limit: number }>(
      `/api/businesses/${bid}/leads?page=${page}&limit=${limit}&search=${encodeURIComponent(search)}`
    ),
  listPrompts: (bid: string) =>
    request<ProjectPrompt[]>(`/api/businesses/${bid}/prompts`),
  createPrompt: (bid: string, data: Partial<ProjectPrompt>) =>
    request<ProjectPrompt>(`/api/businesses/${bid}/prompts`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  listSales: (bid: string) =>
    request<SalesAssignment[]>(`/api/businesses/${bid}/sales`),
  upsertSales: (bid: string, data: Partial<SalesAssignment>) =>
    request<SalesAssignment>(`/api/businesses/${bid}/sales`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  toggleSales: (id: string) =>
    request<{ ok: boolean }>(`/api/sales/${id}/toggle`, { method: "PATCH" }),
  listMessages: (leadId: string) =>
    request<ChatMessage[]>(`/api/leads/${leadId}/messages`),
};
