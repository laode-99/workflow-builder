"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import * as icons from "lucide-react";
import {
  Activity, Calendar, ChevronDown, ChevronRight, ChevronUp, Clock, Database, FileCode2, History, Key,
  LayoutDashboard, Play, Plus, Power, Save, ShieldCheck, Square, Trash2, Zap,  Loader2, Maximize2, Monitor, MoreVertical, MousePointer2, 
  CheckCircle2, XCircle, RefreshCw, Server, Shield, List, Terminal, X, Variable, AlertCircle, Workflow, Settings, Search, Send, Smartphone, User, Network, Phone
} from "lucide-react";
import { api, Business, Workflow as WorkflowType, Credential, Execution, RegistryItem, ExecutionLog, Step } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger, DialogClose } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Toaster, toast } from "sonner";

type TabRoute = "workspace" | "credentials" | "logs";

const cronPresets = [
  { label: "Manual (Run Now only)", value: "" },
  { label: "Every 10 Minutes", value: "*/10 * * * *" },
  { label: "Every Hour", value: "0 * * * *" },
  { label: "Every Day at Midnight", value: "0 0 * * *" },
  { label: "Every Morning (9:00 AM)", value: "0 9 * * *" },
  { label: "Every Monday at Midnight", value: "0 0 * * 1" },
];

// ==================== Components ====================

const WorkflowFlow = ({ steps, active }: { steps: Step[], active?: boolean }) => {
  if (!steps || steps.length === 0) return null;

  return (
    <div className="flex items-center gap-0 my-4 py-4 px-2">
      {steps.map((step: Step, i: number) => {
        const Icon = (icons as any)[step.icon] || icons.Zap;
        return (
          <div key={step.id} className="group relative flex items-center">
            <div className="flex flex-col items-center">
              <div className={`w-8 h-8 rounded-lg flex items-center justify-center transition-all ${active ? "bg-blue-600 text-white shadow-lg ring-2 ring-blue-100" : "bg-slate-50 text-slate-400 border"}`}>
                <Icon className="w-4 h-4" />
              </div>
              <span className="text-[8px] font-bold mt-1 text-slate-400 uppercase tracking-tighter truncate w-12 text-center">{step.label}</span>
            </div>
            {i < steps.length - 1 && (
              <div className={`w-6 h-[1px] mb-4 ${active ? "bg-blue-300" : "bg-slate-200"}`} />
            )}
          </div>
        );
      })}
    </div>
  );
};

const DeploymentManifest = ({ wf, credentials }: { wf: WorkflowType, credentials: Credential[] }) => {
  if (!wf.params || wf.params.length === 0) return null;
  
  let vars: Record<string, string> = {};
  try { vars = JSON.parse(wf.variables || "{}"); } catch { return null; }
  
  const activeParams = wf.params.filter((p: any) => !!vars[p.key]);
  if (activeParams.length === 0 && !wf.stop_time) return null;

  return (
    <div className="mt-3 bg-slate-50/80 border border-slate-100 rounded-lg p-2.5 shadow-inner">
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-1.5 text-slate-400">
          <Server className="w-3 h-3" />
          <span className="text-[9px] font-bold uppercase tracking-widest leading-none">Verified Deployment Logic</span>
        </div>
        {wf.stop_time && (
          <Badge variant="outline" className="text-[8px] h-4 bg-amber-50 text-amber-700 border-amber-200 uppercase px-1 shadow-sm">
            📍 Stop {wf.stop_time} WIB
          </Badge>
        )}
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-1.5 opacity-90">
        {activeParams.map(p => {
          const val = vars[p.key];
          let displayVal = val;
          if (p.type === "credential") {
            const cred = credentials.find(c => c.id === val);
            displayVal = cred ? cred.label : "Invalid Key";
          }
          return (
            <div key={p.key} className="flex items-center justify-between text-[10px] py-0.5 border-b border-slate-100/50 last:border-0 overflow-hidden">
              <span className="text-slate-500 font-medium truncate pr-2">{p.key.replace(/_/g, " ")}:</span>
              <span className={`font-bold truncate ${p.type === "credential" ? "text-blue-600" : "text-slate-700"}`}>
                {p.type === "credential" && "🔑 "}{displayVal}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
};

export default function DashboardPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  const [route, setRouteState] = useState<TabRoute>((searchParams.get("tab") as TabRoute) || "workspace");
  const [businesses, setBusinesses] = useState<Business[]>([]);
  const [activeBiz, setActiveBizState] = useState<string | null>(searchParams.get("biz"));
  const [loading, setLoading] = useState(true);

  const [workflows, setWorkflows] = useState<WorkflowType[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [executions, setExecutions] = useState<Execution[]>([]);
  const [registry, setRegistry] = useState<RegistryItem[]>([]);

  // Forms
  const [newBizName, setNewBizName] = useState("");
  const [newCredLabel, setNewCredLabel] = useState("");
  const [newCredIntegration, setNewCredIntegration] = useState("retell_ai");
  const [newCredValue, setNewCredValue] = useState("");
  const [newCredIsGlobal, setNewCredIsGlobal] = useState(false);
  const [isAddingCred, setIsAddingCred] = useState(false);
  const [newWfAlias, setNewWfAlias] = useState("");
  const [selectedSignature, setSelectedSignature] = useState("");
  const [newWfCron, setNewWfCron] = useState("");
  const [cronMode, setCronMode] = useState<"preset" | "custom">("preset");
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [configuringWf, setConfiguringWf] = useState<WorkflowType | null>(null);

  // Workflow UI state
  const [expandedWf, setExpandedWf] = useState<string | null>(null);
  const [editingVars, setEditingVars] = useState<Record<string, string>>({});
  const [savingVars, setSavingVars] = useState(false);
  const [editingCron, setEditingCron] = useState<string>("");
  const [savingCron, setSavingCron] = useState(false);
  const [editingStopTime, setEditingStopTime] = useState<string>("");
  const [savingStopTime, setSavingStopTime] = useState(false);
  const [triggeringWf, setTriggeringWf] = useState<string | null>(null);
  const [menuOpenWf, setMenuOpenWf] = useState<string | null>(null);

  // Preview State
  const [isPreviewing, setIsPreviewing] = useState<string | null>(null);
  const [previewData, setPreviewData] = useState<any[] | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);

  const loadPreview = useCallback(async (workflow: WorkflowType, vars: Record<string, string>) => {
    setIsPreviewing(workflow.id);
    setPreviewData(null);
    setPreviewError(null);
    try {
      const sheetsCred = credentials.find(c => c.integration === "google_sheets");
      if (!sheetsCred) throw new Error("No Google Sheets credential found. Please add one first.");

      const sheetId = vars["google_sheet_id"];
      const tabName = vars["google_sheet_tab_name"];

      if (!sheetId || !tabName) throw new Error("Missing 'google_sheet_id' or 'google_sheet_tab_name'.");

      const data = await api.previewCredentialData(sheetsCred.id, sheetId, tabName);
      setPreviewData(data);
    } catch (e) {
      setPreviewError((e as Error).message);
    } finally {
      setIsPreviewing(null);
    }
  }, [credentials]);

  // Log Viewer State
  const [viewingLogs, setViewingLogs] = useState<string | null>(null);
  const [executionLogs, setExecutionLogs] = useState<ExecutionLog[]>([]);
  const [loadingLogs, setLoadingLogs] = useState(false);

  // Verification state
  const [isVerifying, setIsVerifying] = useState(false);
  const [verifyResult, setVerifyResult] = useState<{ ok: boolean; msg?: string } | null>(null);

  // Flow Visibility
  const [visibleFlows, setVisibleFlows] = useState<Record<string, boolean>>({});

  // Logic Blueprint State
  const [viewingBlueprint, setViewingBlueprint] = useState<WorkflowType | null>(null);
  const [logicContent, setLogicContent] = useState<string>("");
  const [loadingLogic, setLoadingLogic] = useState(false);
  const [activeVerifyId, setActiveVerifyId] = useState<string | null>(null);
  const [credStatus, setCredStatus] = useState<Record<string, { ok: boolean; msg?: string }>>({});

  // URL sync
  const updateURL = useCallback((biz: string | null, tab: TabRoute) => {
    const params = new URLSearchParams();
    if (biz) params.set("biz", biz);
    params.set("tab", tab);
    router.replace(`/?${params.toString()}`, { scroll: false });
  }, [router]);

  const setRoute = useCallback((tab: TabRoute) => {
    setRouteState(tab);
    updateURL(activeBiz, tab);
  }, [activeBiz, updateURL]);

  const setActiveBiz = useCallback((biz: string) => {
    setActiveBizState(biz);
    updateURL(biz, route);
  }, [route, updateURL]);

  // Load businesses
  useEffect(() => {
    Promise.all([api.listBusinesses(), api.getRegistry()])
      .then(([biz, reg]) => {
        setBusinesses(biz || []);
        setRegistry(reg || []);
        const urlBiz = searchParams.get("biz");
        if (urlBiz && biz?.some(b => b.id === urlBiz)) {
          setActiveBizState(urlBiz);
        } else if (biz && biz.length > 0) {
          setActiveBizState(biz[0].id);
          updateURL(biz[0].id, (searchParams.get("tab") as TabRoute) || "workspace");
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const activeBizId = activeBiz;
  const refreshData = useCallback(() => {
    if (!activeBizId) return;
    api.listWorkflows(activeBizId).then(w => setWorkflows(w || [])).catch(() => setWorkflows([]));
    api.listCredentials(activeBizId).then(c => setCredentials(c || [])).catch(() => setCredentials([]));
    api.listExecutions(activeBizId).then(e => setExecutions(e || [])).catch(() => setExecutions([]));
  }, [activeBizId]);

  const loadLogs = useCallback(async (id: string) => {
    setViewingLogs(id);
    setLoadingLogs(true);
    try {
      const data = await api.getExecutionLogs(id);
      setExecutionLogs(data || []);
    } catch (e) {
      setExecutionLogs([]);
    } finally {
      setLoadingLogs(false);
    }
  }, []);

  const loadBlueprint = useCallback(async (wf: WorkflowType) => {
    setViewingBlueprint(wf);
    setLoadingLogic(true);
    setLogicContent("");
    try {
      const res = await api.getWorkflowLogic(wf.id);
      setLogicContent(res.content);
    } catch (e) {
      setLogicContent("Logic documentation not found for this workflow.");
    } finally {
      setLoadingLogic(false);
    }
  }, []);

  const toggleFlow = (id: string) => {
    setVisibleFlows(prev => ({ ...prev, [id]: !prev[id] }));
  };

  useEffect(() => { refreshData(); }, [refreshData]);

  // Auto-poll when any workflow has a running execution
  const hasRunning = executions.some(e => e.status === "running" || e.status === "queued");
  const pollRef = useRef<ReturnType<typeof setInterval>>();
  useEffect(() => {
    if (hasRunning) {
      pollRef.current = setInterval(refreshData, 3000);
    }
    return () => { if (pollRef.current) clearInterval(pollRef.current); };
  }, [hasRunning, refreshData]);

  // Close menu when clicking outside
  useEffect(() => {
    const handler = () => setMenuOpenWf(null);
    if (menuOpenWf) document.addEventListener("click", handler);
    return () => document.removeEventListener("click", handler);
  }, [menuOpenWf]);

  // ======================== Actions ========================

  const createBusiness = async () => {
    if (!newBizName.trim()) return;
    const b = await api.createBusiness(newBizName.trim());
    setBusinesses(prev => [b, ...prev]);
    setActiveBiz(b.id);
    setNewBizName("");
  };

  const deleteBusiness = async (id: string) => {
    if (!confirm("Delete this business and ALL its workflows, credentials, and execution history?")) return;
    await api.deleteBusiness(id);
    setBusinesses(prev => prev.filter(b => b.id !== id));
    if (activeBiz === id) {
      const remaining = businesses.filter(b => b.id !== id);
      setActiveBizState(remaining.length > 0 ? remaining[0].id : null);
    }
  };

  const selectedReg = registry.find(r => r.signature === selectedSignature);

  const createWorkflow = async () => {
    if (!activeBiz || !selectedSignature || !newWfAlias.trim()) return;
    await api.createWorkflow(activeBiz, { signature: selectedSignature, alias: newWfAlias.trim(), trigger_cron: newWfCron });
    setSelectedSignature(""); setNewWfAlias(""); setNewWfCron(""); setCronMode("preset");
    setAddDialogOpen(false);
    refreshData();
  };

  const deleteWorkflow = async (id: string) => {
    if (!confirm("Delete this workflow and all its execution history?")) return;
    await api.deleteWorkflow(id);
    setMenuOpenWf(null);
    refreshData();
  };

  const triggerWorkflow = async (id: string) => {
    setTriggeringWf(id);
    try {
      await api.triggerWorkflow(id);
      refreshData();
    } finally {
      setTimeout(() => setTriggeringWf(null), 1000);
    }
  };

  const stopWorkflow = async (id: string) => {
    await api.stopWorkflow(id);
    refreshData();
  };

  const openVarEditor = (wf: WorkflowType) => {
    if (expandedWf === wf.id) {
      setExpandedWf(null);
    } else {
      setExpandedWf(wf.id);
      setEditingVars(JSON.parse(wf.variables || "{}"));
      setEditingCron(wf.trigger_cron || "");
      setEditingStopTime(wf.stop_time || "");
    }
  };

  const saveVars = async (id: string) => {
    setSavingVars(true);
    await api.updateWorkflowVars(id, JSON.stringify(editingVars));
    setSavingVars(false);
    refreshData();
  };

  const saveCron = async (id: string) => {
    setSavingCron(true);
    await api.updateWorkflowCron(id, editingCron);
    setSavingCron(false);
    refreshData();
    toast.success("Schedule updated");
  };

  const saveStopTime = async (id: string) => {
    setSavingStopTime(true);
    await api.updateWorkflowStopTime(id, editingStopTime);
    setSavingStopTime(false);
    refreshData();
    toast.success("Stop time updated");
  };

  const toggleWfActive = async (id: string) => {
    await api.toggleWorkflow(id);
    refreshData();
    toast.success("Workflow status updated");
  };

  const addCredential = async () => {
    if (!newCredLabel || !newCredValue) return;
    try {
      if (!activeBiz) return;
      await api.createCredential(activeBiz, {
        label: newCredLabel,
        integration: newCredIntegration,
        data: newCredValue,
        is_global: newCredIsGlobal,
      });
      toast.success("Credential added");
      setNewCredLabel("");
      setNewCredValue("");
      setNewCredIsGlobal(false);
      setIsAddingCred(false);
      refreshData();
    } catch (e) {
      toast.error("Failed to add credential");
    }
  };

  const deleteCredential = async (id: string) => {
    await api.deleteCredential(id);
    refreshData();
  };

  const startVerification = (id: string) => {
    setActiveVerifyId(id);
    setVerifyResult(null);
    setIsVerifying(true);
    
    // Call API
    api.verifyCredential(id)
      .then(() => {
        setVerifyResult({ ok: true });
        setCredStatus(prev => ({ ...prev, [id]: { ok: true } }));
      })
      .catch((err: any) => {
        setVerifyResult({ ok: false, msg: err.message || "Invalid Key" });
        setCredStatus(prev => ({ ...prev, [id]: { ok: false, msg: err.message || "Invalid Key" } }));
      })
      .finally(() => {
        setIsVerifying(false);
      });
  };

  // ======================== Helpers ========================

  const activeBusiness = businesses.find(b => b.id === activeBiz);

  const getLatestExec = (wfId: string): Execution | undefined =>
    executions.find(e => e.workflow_id === wfId);

  const isWfRunning = (wfId: string) => {
    const ex = getLatestExec(wfId);
    return ex && (ex.status === "running" || ex.status === "queued");
  };
  const areVarsConfigured = (wf: WorkflowType): boolean => {
    if (!wf.params || wf.params.length === 0) return true;
    try {
      const vars = JSON.parse(wf.variables || "{}");
      return wf.params.every((p: any) => !!vars[p.key]);
    } catch { return false; }
  };

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-50">
        <Loader2 className="w-8 h-8 animate-spin text-blue-600" />
      </div>
    );
  }

  // ======================== RENDER ========================
  return (
    <div className="flex h-screen bg-slate-50 font-sans">

      {/* ===== SIDEBAR ===== */}
      <aside className="w-60 border-r bg-white flex flex-col shrink-0">
        <div className="p-5 border-b">
          <div className="flex items-center gap-2.5">
            <div className="w-7 h-7 rounded-md bg-blue-600 flex items-center justify-center">
              <Zap className="w-3.5 h-3.5 text-white" />
            </div>
            <span className="font-bold text-base tracking-tight">FlowBuilder</span>
          </div>
        </div>

        <nav className="p-3 space-y-0.5">
          {[
            { key: "workspace" as const, icon: LayoutDashboard, label: "Workflows" },
            { key: "credentials" as const, icon: Key, label: "Credentials" },
            { key: "logs" as const, icon: History, label: "Execution Logs" },
          ].map(item => (
            <button
              key={item.key}
              onClick={() => setRoute(item.key)}
              className={`w-full flex items-center gap-2.5 px-3 py-2 rounded-md text-sm transition-colors ${
                route === item.key ? "bg-blue-50 text-blue-700 font-medium" : "text-slate-600 hover:bg-slate-50"
              }`}
            >
              <item.icon className="w-4 h-4" /> {item.label}
            </button>
          ))}
        </nav>

        <div className="px-3 mt-4 flex-1 overflow-auto">
          <p className="text-[10px] font-semibold text-slate-400 uppercase tracking-wider px-3 mb-2">Businesses</p>
          <div className="flex gap-1 mb-2 px-1">
            <Input
              value={newBizName}
              onChange={e => setNewBizName(e.target.value)}
              placeholder="New business..."
              className="h-7 text-xs"
              onKeyDown={e => e.key === "Enter" && createBusiness()}
            />
            <Button size="sm" className="h-7 w-7 p-0 shrink-0" onClick={createBusiness} disabled={!newBizName.trim()}>
              <Plus className="w-3.5 h-3.5" />
            </Button>
          </div>
          <div className="space-y-0.5">
            {businesses.map(b => (
              <div key={b.id} className="group flex items-center">
                <button
                  onClick={() => { setActiveBiz(b.id); setRoute("workspace"); }}
                  className={`flex-1 flex items-center gap-2 px-3 py-1.5 rounded-md text-sm truncate transition-colors ${
                    activeBiz === b.id ? "bg-slate-100 font-medium text-slate-900" : "text-slate-500 hover:bg-slate-50"
                  }`}
                >
                  <Database className={`w-3.5 h-3.5 shrink-0 ${activeBiz === b.id ? "text-blue-600" : "text-slate-400"}`} />
                  <span className="truncate">{b.name}</span>
                </button>
                <button
                  onClick={(e) => { e.stopPropagation(); deleteBusiness(b.id); }}
                  className="opacity-0 group-hover:opacity-100 p-1 rounded hover:bg-red-50 transition-opacity shrink-0"
                  title="Delete business"
                >
                  <Trash2 className="w-3 h-3 text-red-400 hover:text-red-600" />
                </button>
              </div>
            ))}
            {businesses.length === 0 && <p className="text-xs text-slate-400 px-3 py-4">No businesses yet.</p>}
          </div>
        </div>

        <div className="p-3 border-t">
          <div className="flex items-center gap-2.5 px-2">
            <div className="w-7 h-7 rounded-full bg-blue-100 flex items-center justify-center font-bold text-blue-700 text-[10px]">LM</div>
            <div>
              <p className="text-xs font-medium leading-none">La Ode M.</p>
              <p className="text-[10px] text-slate-400">Admin</p>
            </div>
          </div>
        </div>
      </aside>

      {/* ===== MAIN ===== */}
      <main className="flex-1 overflow-auto flex flex-col min-w-0">

        <header className="border-b bg-white shrink-0">
          <div className="flex items-center justify-between px-6 h-12">
            <div className="flex items-center gap-1.5 text-sm">
              <span className="text-slate-400">Businesses</span>
              <ChevronRight className="w-3 h-3 text-slate-300" />
              <span className="font-semibold text-slate-800">{activeBusiness?.name || "—"}</span>
            </div>
          </div>
          {activeBiz && (
            <div className="px-6 flex gap-0 border-t bg-slate-50/50">
              {[
                { key: "workspace" as const, label: "Workflows" },
                { key: "credentials" as const, label: "Credentials" },
                { key: "logs" as const, label: "Execution Logs" },
              ].map(tab => (
                <button
                  key={tab.key}
                  onClick={() => setRoute(tab.key)}
                  className={`px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
                    route === tab.key
                      ? "border-blue-600 text-blue-700"
                      : "border-transparent text-slate-500 hover:text-slate-700"
                  }`}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          )}
        </header>

        <div className="flex-1 overflow-auto p-6">
          {!activeBiz ? (
            <div className="flex flex-col items-center justify-center h-full text-slate-400">
              <Database className="w-10 h-10 mb-3 opacity-20" />
              <p className="text-sm">Create or select a business to begin.</p>
            </div>
          ) : (
            <div className="max-w-4xl mx-auto">

              {/* ===================== WORKFLOWS TAB ===================== */}
              {route === "workspace" && (
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h2 className="text-lg font-bold">Workflows</h2>
                      <p className="text-xs text-slate-500 mt-0.5">Add a workflow template, configure its variables, and run it.</p>
                    </div>

                    {/* ========== ADD WORKFLOW DIALOG ========== */}
                    <Dialog open={addDialogOpen} onOpenChange={setAddDialogOpen}>
                      <DialogTrigger render={
                        <Button size="sm" className="bg-blue-600 hover:bg-blue-700 text-white">
                          <Plus className="w-4 h-4 mr-1.5" /> Add Workflow
                        </Button>
                      } />
                      <DialogContent className="max-w-lg bg-white border shadow-2xl">
                        <DialogHeader>
                          <DialogTitle className="text-lg">Add a Workflow</DialogTitle>
                          <DialogDescription className="text-slate-500">
                            Workflows are Go functions that run your business logic. Pick one from the available templates below, give it a name, and configure it.
                          </DialogDescription>
                        </DialogHeader>

                        <div className="space-y-5 pt-2">
                          {/* Step 1 — pick template */}
                          <div>
                            <label className="text-xs font-semibold text-slate-700 mb-2 block">Workflow Template</label>
                            {registry.length === 0 ? (
                              <div className="bg-amber-50 border border-amber-200 rounded-lg p-3 text-sm text-amber-700">
                                No workflow templates available. Register one in your Go code first.
                              </div>
                            ) : (
                              <div className="space-y-2">
                                {registry.map(r => (
                                  <button
                                    key={r.signature}
                                    onClick={() => setSelectedSignature(r.signature)}
                                    className={`w-full text-left border rounded-lg p-3.5 transition-all ${
                                      selectedSignature === r.signature
                                        ? "border-blue-500 bg-blue-50/50 ring-1 ring-blue-200"
                                        : "border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-50"
                                    }`}
                                  >
                                    <div className="flex items-center justify-between">
                                      <span className="font-semibold text-sm">{r.name}</span>
                                      <code className="text-[10px] text-slate-400 font-mono">{r.signature}</code>
                                    </div>
                                    <p className="text-xs text-slate-500 mt-1 leading-relaxed">{r.description}</p>
                                    {r.params && r.params.length > 0 && (
                                      <div className="flex flex-wrap gap-1 mt-2">
                                        {r.params.map((p: any) => (
                                          <span key={p.key} className="text-[10px] bg-slate-100 text-slate-600 px-1.5 py-0.5 rounded font-mono">
                                            {p.key}{p.type === "credential" ? ":🔑" : ""}
                                          </span>
                                        ))}
                                      </div>
                                    )}
                                  </button>
                                ))}
                              </div>
                            )}
                          </div>

                          {/* Step 2 — name */}
                          {selectedSignature && (
                            <>
                              <div>
                                <label className="text-xs font-semibold text-slate-700 mb-1.5 block">Display Name</label>
                                <Input
                                  value={newWfAlias}
                                  onChange={e => setNewWfAlias(e.target.value)}
                                  placeholder="e.g. Morning Cold Calls"
                                />
                              </div>

                              {/* Step 3 — schedule */}
                              <div>
                                <label className="text-xs font-semibold text-slate-700 mb-1.5 block">Schedule</label>
                                <select
                                  className="w-full border border-slate-200 rounded-lg px-3 py-2.5 text-sm bg-white focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
                                  value={cronMode === "custom" ? "__custom__" : newWfCron}
                                  onChange={e => {
                                    if (e.target.value === "__custom__") {
                                      setCronMode("custom");
                                      setNewWfCron("");
                                    } else {
                                      setCronMode("preset");
                                      setNewWfCron(e.target.value);
                                    }
                                  }}
                                >
                                  {cronPresets.map((p: any) => (
                                    <option key={p.value} value={p.value}>{p.label}</option>
                                  ))}
                                  <option value="__custom__">Custom Cron...</option>
                                </select>
                                {cronMode === "custom" && (
                                  <Input
                                    value={newWfCron}
                                    onChange={e => setNewWfCron(e.target.value)}
                                    placeholder="e.g. 30 9 * * 1-5"
                                    className="mt-2 font-mono"
                                  />
                                )}
                                <p className="text-[11px] text-slate-400 mt-1.5">
                                  {newWfCron ? <>Schedule: <code className="bg-slate-100 px-1 rounded">{newWfCron}</code></> : "No schedule — you\u2019ll trigger this workflow manually."}
                                </p>
                              </div>

                              {/* Submit */}
                              <Button
                                className="w-full bg-blue-600 hover:bg-blue-700 h-10 text-sm font-medium"
                                onClick={createWorkflow}
                                disabled={!newWfAlias.trim()}
                              >
                                <Plus className="w-4 h-4 mr-2" /> Create Workflow
                              </Button>
                            </>
                          )}
                        </div>
                      </DialogContent>
                    </Dialog>
                  </div>

                  {/* Empty state */}
                  {workflows.length === 0 ? (
                    <Card className="border-dashed border-2 border-slate-200 bg-white">
                      <CardContent className="flex flex-col items-center justify-center py-16 text-slate-400">
                        <FileCode2 className="w-10 h-10 mb-3 opacity-30" />
                        <p className="text-sm font-medium">No workflows yet</p>
                        <p className="text-xs mt-1 max-w-xs text-center">Click &quot;Add Workflow&quot; to create one from your available Go templates.</p>
                      </CardContent>
                    </Card>
                  ) : (
                    /* ========== WORKFLOW CARDS ========== */
                    workflows.map(wf => {
                      const running = isWfRunning(wf.id);
                      const latestExec = getLatestExec(wf.id);
                      const configured = areVarsConfigured(wf);
                      const triggering = triggeringWf === wf.id;

                      return (
                        <Card key={wf.id} className={`shadow-sm transition-all overflow-visible ${running ? "border-blue-300 ring-1 ring-blue-200" : "border-slate-200"}`}>
                          {/* Color bar */}
                          <div className={`h-1 rounded-t-xl ${running ? "bg-blue-500 animate-pulse" : wf.is_active ? "bg-green-500" : "bg-slate-200"}`} />

                          {/* Main row */}
                          <div className="px-5 py-4 flex items-start justify-between gap-4">
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-2 mb-1.5">
                                <h3 className="font-bold text-base truncate">{wf.alias}</h3>
                                  <button 
                                    onClick={() => toggleWfActive(wf.id)}
                                    className={`flex items-center gap-1.5 px-2 py-0.5 rounded-full border transition-all ${
                                      wf.is_active 
                                        ? "bg-green-50 border-green-200 text-green-700 hover:bg-green-100" 
                                        : "bg-slate-50 border-slate-200 text-slate-500 hover:bg-slate-100"
                                    }`}
                                  >
                                    <Power className={`w-2.5 h-2.5 ${wf.is_active ? "fill-green-600" : ""}`} />
                                    <span className="text-[10px] font-bold uppercase tracking-tight">{wf.is_active ? "Active" : "Paused"}</span>
                                  </button>
                                {running ? (
                                  <Badge className="bg-blue-100 text-blue-700 border-none text-[10px] px-1.5 py-0 leading-4 gap-1">
                                    <Loader2 className="w-2.5 h-2.5 animate-spin" /> RUNNING
                                  </Badge>
                                ) : latestExec?.status === "completed" ? (
                                  <Badge className="bg-green-100 text-green-700 border-none text-[10px] px-1.5 py-0 leading-4">COMPLETED</Badge>
                                ) : latestExec?.status === "failed" ? (
                                  <Badge className="bg-red-100 text-red-700 border-none text-[10px] px-1.5 py-0 leading-4">FAILED</Badge>
                                ) : (
                                  <Badge variant="secondary" className="text-[10px] px-1.5 py-0 leading-4">IDLE</Badge>
                                )}
                              </div>
                              <div className="flex items-center gap-3 text-xs text-slate-400">
                                <span className="font-mono flex items-center gap-1"><FileCode2 className="w-3 h-3" />{wf.signature}</span>
                                {wf.trigger_cron ? (
                                  <span className="flex items-center gap-1"><Clock className="w-3 h-3 text-blue-500" /><code className="bg-slate-100 px-1.5 py-0.5 rounded text-[11px]">{wf.trigger_cron}</code></span>
                                ) : (
                                  <span className="flex items-center gap-1 text-slate-300"><Clock className="w-3 h-3" /> Manual</span>
                                )}
                              </div>
                              
                              {/* Deployment Manifest (Source of Truth) */}
                              <DeploymentManifest wf={wf} credentials={credentials} />
                            </div>

                            <div className="flex items-center gap-2 shrink-0 pt-1">
                              {running ? (
                                <Button size="sm" variant="outline" onClick={() => stopWorkflow(wf.id)} className="text-red-600 border-red-200 hover:bg-red-50 text-[11px] font-bold h-8">
                                  STOP
                                </Button>
                              ) : (
                                <Button 
                                  size="sm" 
                                  className="bg-slate-900 hover:bg-slate-800 text-white shadow-md transition-all active:scale-95 text-[11px] font-bold h-8"
                                  onClick={() => triggerWorkflow(wf.id)}
                                  disabled={triggering || !configured || !wf.is_active}
                                >
                                  {triggering ? <Loader2 className="w-3.5 h-3.5 animate-spin mr-1.5" /> : <Play className="w-3.5 h-3.5 mr-1.5 fill-current" />}
                                  {triggering ? "STARTING..." : "RUN NOW"}
                                </Button>
                              )}
                              
                              <div className="relative group/menu">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className={`h-8 w-8 p-0 ${menuOpenWf === wf.id ? "bg-slate-100" : ""}`}
                                  onClick={(e) => { e.stopPropagation(); setMenuOpenWf(menuOpenWf === wf.id ? null : wf.id); }}
                                >
                                  <MoreVertical className="w-4 h-4" />
                                </Button>
                                
                                {menuOpenWf === wf.id && (
                                  <div className="absolute right-0 top-full mt-1 w-40 bg-white border border-slate-200 rounded-lg shadow-xl z-50 py-1.5 animate-in fade-in zoom-in-95 duration-100">
                                    <button 
                                      onClick={() => openVarEditor(wf)}
                                      className="w-full text-left px-3 py-1.5 text-xs hover:bg-slate-50 flex items-center gap-2"
                                    >
                                      <Settings className="w-3.5 h-3.5 text-slate-400" /> {expandedWf === wf.id ? "Hide Settings" : "Configure"}
                                    </button>
                                    <button 
                                      onClick={() => loadBlueprint(wf)}
                                      className="w-full text-left px-3 py-1.5 text-xs hover:bg-slate-50 flex items-center gap-2"
                                    >
                                      <Shield className="w-3.5 h-3.5 text-blue-500" /> View Blueprint
                                    </button>
                                    <div className="h-px bg-slate-100 my-1" />
                                    <button 
                                      onClick={() => deleteWorkflow(wf.id)}
                                      className="w-full text-left px-3 py-1.5 text-xs hover:bg-red-50 text-red-600 flex items-center gap-2"
                                    >
                                      <Trash2 className="w-3.5 h-3.5" /> Delete
                                    </button>
                                  </div>
                                )}
                              </div>
                            </div>
                          </div>

                          {/* Config panel */}
                          {expandedWf === wf.id && (
                            <div className="border-t bg-slate-50 px-5 py-4">
                              <div className="flex items-center justify-between mb-3">
                                <h4 className="text-xs font-semibold text-slate-500 uppercase tracking-wider">Runtime Variables</h4>
                                <Button size="sm" onClick={() => saveVars(wf.id)} disabled={savingVars} className="text-xs h-7 bg-blue-600 hover:bg-blue-700">
                                  <Save className="w-3.5 h-3.5 mr-1" /> {savingVars ? "Saving..." : "Save"}
                                </Button>
                              </div>
                              {wf.params && wf.params.length > 0 ? (
                                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                                  {wf.params.map((param: any) => (
                                    <div key={param.key} className="space-y-1.5">
                                      <label className="text-[11px] font-bold uppercase tracking-wider text-slate-500 block">
                                        {param.key.replace(/_/g, " ")}
                                        {param.type === "credential" && <span className="ml-1.5 text-blue-500">🔑</span>}
                                      </label>
                                      
                                      {param.type === "credential" ? (
                                        <select
                                          className="w-full h-9 px-3 bg-white border border-slate-200 rounded-md text-sm outline-none focus:ring-2 focus:ring-blue-500 transition-all"
                                          value={editingVars[param.key] || ""}
                                          onChange={e => setEditingVars(prev => ({ ...prev, [param.key]: e.target.value }))}
                                        >
                                          <option value="">Select a Credential...</option>
                                          {credentials
                                            .filter(c => c.integration === param.integration)
                                            .map(c => (
                                              <option key={c.id} value={c.id}>
                                                {c.label} {c.is_global ? "(Global)" : ""}
                                              </option>
                                            ))}
                                        </select>
                                      ) : (
                                        <Input
                                          value={editingVars[param.key] || ""}
                                          onChange={e => setEditingVars(prev => ({ ...prev, [param.key]: e.target.value }))}
                                          placeholder={`Enter ${param.key.replace(/_/g, " ")}...`}
                                          className="text-sm h-9"
                                        />
                                      )}
                                      {param.description && <p className="text-[10px] text-slate-400 italic">{param.description}</p>}
                                    </div>
                                  ))}
                                </div>
                              ) : (
                                <p className="text-xs text-slate-400">This workflow has no configurable variables.</p>
                              )}
                              <p className="text-[11px] text-slate-400 mt-3">
                                Injected at runtime via <code className="bg-white px-1 py-0.5 rounded border text-[10px]">exec.GetVar(&quot;key&quot;)</code>
                              </p>

                              <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mt-6 pt-6 border-t border-slate-200">
                                {/* Stop Time */}
                                <div className="space-y-3">
                                  <div className="flex items-center justify-between">
                                    <h4 className="text-[10px] font-bold text-slate-400 uppercase tracking-tighter flex items-center gap-1.5">
                                      <Clock className="w-3 h-3 text-amber-500" /> Performance & Timing
                                    </h4>
                                    <div className="flex gap-1.5">
                                      {editingStopTime && (
                                        <Button 
                                          size="sm" 
                                          variant="ghost" 
                                          onClick={() => { setEditingStopTime(""); setTimeout(() => saveStopTime(wf.id), 0); }} 
                                          className="h-6 text-[9px] text-slate-400 hover:text-red-500"
                                        >
                                          RESET
                                        </Button>
                                      )}
                                      <Button 
                                        size="sm" 
                                        onClick={() => saveStopTime(wf.id)} 
                                        disabled={savingStopTime} 
                                        className="h-6 text-[9px] bg-amber-600 hover:bg-amber-700 font-bold"
                                      >
                                        <Save className="w-2.5 h-2.5 mr-1" /> {savingStopTime ? "SAVING..." : "SAVE STOP TIME"}
                                      </Button>
                                    </div>
                                  </div>
                                  <div className="bg-white border rounded-lg p-3 shadow-sm">
                                    <label className="text-[10px] font-bold text-slate-500 uppercase mb-1.5 block">Jakarta Stop Time (WIB)</label>
                                    <Input
                                      type="time"
                                      value={editingStopTime}
                                      onChange={e => setEditingStopTime(e.target.value)}
                                      className="h-9 text-sm font-mono"
                                    />
                                    <p className="text-[9px] text-slate-400 mt-2 leading-relaxed italic">
                                      Workflow will gracefully shut down if reached. Use <code className="bg-slate-100 px-1 rounded">00:00</code> or RESET to disable.
                                    </p>
                                  </div>
                                </div>

                                {/* Schedule */}
                                <div className="space-y-3">
                                  <div className="flex items-center justify-between">
                                    <h4 className="text-[10px] font-bold text-slate-400 uppercase tracking-tighter flex items-center gap-1.5">
                                      <Calendar className="w-3 h-3 text-blue-500" /> Automation Schedule
                                    </h4>
                                    <Button 
                                      size="sm" 
                                      onClick={() => saveCron(wf.id)} 
                                      disabled={savingCron} 
                                      className="h-6 text-[9px] bg-blue-600 hover:bg-blue-700 font-bold"
                                    >
                                      <Save className="w-2.5 h-2.5 mr-1" /> {savingCron ? "SAVING..." : "UPDATE SCHEDULE"}
                                    </Button>
                                  </div>
                                  <div className="bg-white border rounded-lg p-3 shadow-sm space-y-2.5">
                                    <select
                                      className="w-full h-9 px-3 bg-slate-50 border border-slate-200 rounded-md text-xs outline-none focus:ring-2 focus:ring-blue-500 transition-all font-medium"
                                      value={cronPresets.some(p => p.value === editingCron) ? editingCron : "custom"}
                                      onChange={e => setEditingCron(e.target.value)}
                                    >
                                      {cronPresets.map(p => (
                                        <option key={p.value} value={p.value}>{p.label}</option>
                                      ))}
                                      <option value="custom">Custom CRON Expression...</option>
                                    </select>
                                    
                                    <Input
                                      value={editingCron}
                                      onChange={e => setEditingCron(e.target.value)}
                                      placeholder="e.g. 0 9 * * 1-5"
                                      className="h-9 text-xs font-mono"
                                    />
                                  </div>
                                </div>
                              </div>

                              {/* Preview Area */}
                              {wf.signature === "MortgageCallWorkflow" && (
                                <div className="mt-4 pt-4 border-t border-slate-200">
                                  <div className="flex items-center justify-between mb-3">
                                    <span className="text-[10px] font-bold text-slate-400 uppercase tracking-tighter">Data Preview</span>
                                    <Button 
                                      size="sm" 
                                      variant="ghost" 
                                      onClick={() => loadPreview(wf, editingVars)}
                                      disabled={!!isPreviewing}
                                      className="h-6 text-[10px] text-blue-600 hover:bg-blue-50"
                                    >
                                      {isPreviewing ? <Loader2 className="w-3 h-3 animate-spin mr-1" /> : <RefreshCw className="w-3 h-3 mr-1" />}
                                      REFRESH PREVIEW
                                    </Button>
                                  </div>

                                  {previewError && (
                                    <div className="bg-red-50 text-red-600 p-2 rounded text-[10px] flex items-center gap-2">
                                      <AlertCircle className="w-3 h-3" /> {previewError}
                                    </div>
                                  )}

                                  {previewData ? (
                                    <div className="mt-3 border rounded-lg overflow-hidden bg-white shadow-sm overflow-x-auto max-h-[300px]">
                                      {(() => {
                                        const allKeys = Array.from(new Set(previewData.flatMap((row: any) => Object.keys(row))));
                                        const priority = ["Name", "First Name", "Phone Number", "Call Date", "Status", "Summary"];
                                        const headers = [
                                          ...priority.filter((k: string) => allKeys.includes(k)),
                                          ...allKeys.filter((k: string) => !priority.includes(k))
                                        ];

                                        return (
                                          <Table className="min-w-max border-collapse">
                                            <TableHeader className="bg-slate-50 border-b">
                                              <TableRow>
                                                {headers.map((h: string) => (
                                                  <TableHead key={h} className="text-[10px] font-bold text-slate-500 uppercase h-8 px-3">
                                                    {h.replace(/_/g, " ")}
                                                  </TableHead>
                                                ))}
                                              </TableRow>
                                            </TableHeader>
                                            <TableBody>
                                              {previewData.length > 0 ? previewData.map((row: any, idx: number) => (
                                                <TableRow key={idx} className="hover:bg-slate-50 transition-colors border-b last:border-0 h-8">
                                                  {headers.map((h: string) => (
                                                    <TableCell key={h} className="text-[10px] py-1.5 px-3 text-slate-600 truncate max-w-[150px]">
                                                      {row[h] !== undefined && row[h] !== null && String(row[h]).trim() !== "" ? String(row[h]) : <span className="text-slate-300">—</span>}
                                                    </TableCell>
                                                  ))}
                                                </TableRow>
                                              )) : (
                                                <TableRow>
                                                  <TableCell colSpan={headers.length} className="p-8 text-center text-slate-400 italic text-xs">
                                                    No leads found in this range.
                                                  </TableCell>
                                                </TableRow>
                                              )}
                                            </TableBody>
                                          </Table>
                                        );
                                      })()}
                                    </div>
                                  ) : !isPreviewing && !previewError && (
                                    <div className="bg-slate-100/50 rounded-lg py-6 flex flex-col items-center justify-center border border-dashed text-slate-400 italic text-[10px]">
                                      Select Tab & Sheet ID then refresh to preview data
                                    </div>
                                  )}
                                </div>
                              )}
                            </div>
                          )}
                        </Card>
                      );
                    })
                  )}
                </div>
              )}

              {/* ===================== CREDENTIALS TAB ===================== */}
              {route === "credentials" && (
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h2 className="text-lg font-bold">Credentials Vault</h2>
                      <p className="text-xs text-slate-500 mt-0.5">Encrypted API keys used by workflows at runtime.</p>
                    </div>
                    <Dialog open={isAddingCred} onOpenChange={setIsAddingCred}>
                      <DialogTrigger render={
                        <Button size="sm" onClick={() => setIsAddingCred(true)}><Plus className="w-4 h-4 mr-1.5" /> Add Credential</Button>
                      } />
                      <DialogContent className="max-w-md bg-white border shadow-2xl p-0 overflow-hidden">
                        <DialogHeader className="p-6 pb-2">
                          <DialogTitle className="text-lg">Store a Credential</DialogTitle>
                          <DialogDescription className="text-slate-500">The secret value is encrypted with AES-256-GCM and can never be viewed again.</DialogDescription>
                        </DialogHeader>
                        <div className="space-y-4 p-6 pt-2">
                          <div>
                            <label className="text-xs font-semibold text-slate-700 mb-1.5 block">Label</label>
                            <Input value={newCredLabel} onChange={e => setNewCredLabel(e.target.value)} placeholder="e.g. Retell Production Key" />
                          </div>
                          <div>
                            <label className="text-xs font-semibold text-slate-700 mb-1.5 block">Integration Type</label>
                            <select
                              className="w-full border border-slate-200 rounded-lg px-3 py-2.5 text-sm bg-white outline-none focus:ring-2 focus:ring-blue-500"
                              value={newCredIntegration}
                              onChange={e => setNewCredIntegration(e.target.value)}
                            >
                              <option value="retell_ai">Retell AI</option>
                              <option value="google_sheets">Google Sheets (Service Account JSON)</option>
                              <option value="sendgrid">SendGrid</option>
                              <option value="stripe">Stripe</option>
                              <option value="webhook">Custom Webhook</option>
                              <option value="other">Other</option>
                            </select>
                          </div>
                          <div className="space-y-2">
                            <label className="text-xs font-bold uppercase text-slate-500">Security Value</label>
                            <Input
                              type="password"
                              value={newCredValue}
                              onChange={e => setNewCredValue(e.target.value)}
                              placeholder="API Key or Token..."
                            />
                          </div>

                          <div className="flex items-center gap-2 py-2">
                            <input
                              type="checkbox"
                              id="is_global_check"
                              checked={newCredIsGlobal}
                              onChange={e => setNewCredIsGlobal(e.target.checked)}
                              className="w-4 h-4 rounded text-blue-600 focus:ring-blue-500"
                            />
                            <label htmlFor="is_global_check" className="text-xs font-medium text-slate-700">
                              Mark as Global Credential (Share across all businesses)
                            </label>
                          </div>
                        </div>
                        <div className="p-6 bg-slate-50 border-t flex gap-3">
                          <Button className="w-full bg-blue-600 hover:bg-blue-700 h-10" onClick={addCredential}>
                            <ShieldCheck className="w-4 h-4 mr-2" /> Store Encrypted
                          </Button>
                        </div>
                      </DialogContent>
                    </Dialog>
                  </div>

                  {credentials.length === 0 ? (
                    <Card className="border-dashed border-2 border-slate-200">
                      <CardContent className="flex flex-col items-center justify-center py-16 text-slate-400">
                        <ShieldCheck className="w-10 h-10 mb-3 opacity-30" />
                        <p className="text-sm font-medium">No credentials stored</p>
                        <p className="text-xs mt-1">Add API keys for Retell, Google Sheets, etc.</p>
                      </CardContent>
                    </Card>
                  ) : (
                    <div className="space-y-2">
                      {credentials.map(c => (
                        <div key={c.id} className="flex items-center justify-between bg-white border rounded-lg px-4 py-3 shadow-sm group">
                          <div className="flex items-center gap-3">
                            <div className={`w-9 h-9 rounded-md flex items-center justify-center ${
                              credStatus[c.id]?.ok === true ? "bg-green-100" :
                              credStatus[c.id]?.ok === false ? "bg-red-100" :
                              "bg-slate-100"
                            }`}>
                              {credStatus[c.id]?.ok === true ? <CheckCircle2 className="w-4 h-4 text-green-600" /> :
                               credStatus[c.id]?.ok === false ? <XCircle className="w-4 h-4 text-red-600" /> :
                               <ShieldCheck className="w-4 h-4 text-slate-500" />}
                            </div>
                            <div>
                              <p className="font-medium text-sm flex items-center gap-2">
                                {c.label}
                                {c.is_global && (
                                  <Badge className="bg-blue-50 text-blue-600 border-blue-100 text-[9px] px-1.5 h-4 uppercase font-bold">Global</Badge>
                                )}
                                {credStatus[c.id]?.ok === false && (
                                  <span className="text-[10px] text-red-500 font-normal">{credStatus[c.id]?.msg}</span>
                                )}
                              </p>
                              <div className="flex items-center gap-2 mt-0.5">
                                <Badge variant="outline" className="text-[10px] font-mono">{c.integration}</Badge>
                                <span className="text-[10px] text-slate-400">{new Date(c.created_at).toLocaleDateString()}</span>
                              </div>
                            </div>
                          </div>
                          <div className="flex items-center gap-1">
                            <Dialog open={activeVerifyId === c.id} onOpenChange={(open) => !open && setActiveVerifyId(null)}>
                              <DialogTrigger render={
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="h-8 px-2 text-xs gap-1.5 transition-opacity opacity-0 group-hover:opacity-100"
                                  onClick={() => startVerification(c.id)}
                                >
                                  <RefreshCw className="w-3 h-3" /> Verify
                                </Button>
                              } />
                              <DialogContent className="max-w-sm bg-white border shadow-2xl p-0 overflow-hidden">
                                <div className={`h-1.5 w-full ${isVerifying ? "bg-blue-100" : verifyResult?.ok ? "bg-green-500" : "bg-red-500"}`}>
                                  {isVerifying && <div className="h-full bg-blue-600 animate-[progress_2s_ease-in-out_infinite]" style={{ width: "30%" }} />}
                                </div>
                                <div className="p-6">
                                  <div className="flex flex-col items-center text-center">
                                    <div className={`w-16 h-16 rounded-full flex items-center justify-center mb-4 ${
                                      isVerifying ? "bg-blue-50 text-blue-600" :
                                      verifyResult?.ok ? "bg-green-50 text-green-600" :
                                      "bg-red-50 text-red-600"
                                    }`}>
                                      {isVerifying ? <Server className="w-8 h-8 animate-pulse" /> :
                                       verifyResult?.ok ? <CheckCircle2 className="w-8 h-8" /> :
                                       <AlertCircle className="w-8 h-8" />}
                                    </div>
                                    <DialogTitle className="text-xl font-bold mb-1">
                                      {isVerifying ? "Verifying Connection..." :
                                       verifyResult?.ok ? "Credential Valid" :
                                       "Verification Failed"}
                                    </DialogTitle>
                                    <DialogDescription className="text-slate-500 text-sm">
                                      {isVerifying ? `Testing encrypted handshake with ${c.integration}...` :
                                       verifyResult?.ok ? `Successfully authenticated with ${c.integration}. This key is ready for use.` :
                                       verifyResult?.msg || "The provided key could not be authenticated."}
                                    </DialogDescription>

                                    {!isVerifying && (
                                      <div className="mt-6 w-full flex gap-3">
                                        <Button
                                          variant="outline"
                                          className="flex-1"
                                          onClick={() => startVerification(c.id)}
                                        >
                                          <RefreshCw className="w-3.5 h-3.5 mr-2" /> Retry
                                        </Button>
                                        <DialogClose render={
                                          <Button className="flex-1 bg-slate-900 hover:bg-slate-800 text-white font-medium">
                                            Close
                                          </Button>
                                        } />
                                      </div>
                                    )}
                                  </div>
                                </div>
                              </DialogContent>
                            </Dialog>
                            
                            <Button variant="ghost" size="sm" className="opacity-0 group-hover:opacity-100 text-red-400 hover:text-red-700 hover:bg-red-50 h-8 w-8 p-0" onClick={() => deleteCredential(c.id)}>
                              <Trash2 className="w-4 h-4" />
                            </Button>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {/* ===================== EXECUTION LOGS TAB ===================== */}
              {route === "logs" && (
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h2 className="text-lg font-bold">Execution History</h2>
                      <p className="text-xs text-slate-500 mt-0.5">All workflow runs for this business.</p>
                    </div>
                    <Button size="sm" variant="outline" onClick={refreshData} className="text-xs">
                      <Activity className="w-3.5 h-3.5 mr-1.5" /> Refresh
                    </Button>
                  </div>

                  {executions.length === 0 ? (
                    <Card className="border-dashed border-2 border-slate-200">
                      <CardContent className="flex flex-col items-center justify-center py-16 text-slate-400">
                        <Activity className="w-10 h-10 mb-3 opacity-30" />
                        <p className="text-sm font-medium">No executions yet</p>
                        <p className="text-xs mt-1">Runs will appear here once a workflow is triggered.</p>
                      </CardContent>
                    </Card>
                  ) : (
                    <Card className="shadow-sm overflow-hidden">
                      <Table>
                        <TableHeader>
                          <TableRow className="bg-slate-50">
                            <TableHead className="text-xs">ID</TableHead>
                            <TableHead className="text-xs">Workflow</TableHead>
                            <TableHead className="text-xs">Status</TableHead>
                            <TableHead className="text-xs">Started</TableHead>
                            <TableHead className="text-xs">Completed</TableHead>
                            <TableHead className="text-xs"></TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {executions.map((e: Execution) => (
                            <TableRow key={e.id}>
                              <TableCell className="font-mono text-xs text-slate-500">{e.id.slice(0, 8)}</TableCell>
                              <TableCell className="font-medium text-sm">{(e as any).workflow?.alias || e.workflow_id.slice(0, 8)}</TableCell>
                              <TableCell>
                                <Badge
                                  className={`text-[10px] ${
                                    e.status === "completed" ? "bg-green-100 text-green-700 border-none" :
                                    e.status === "failed" ? "bg-red-100 text-red-700 border-none" :
                                    e.status === "running" ? "bg-blue-100 text-blue-700 border-none" :
                                    "bg-slate-100 text-slate-600 border-none"
                                  }`}
                                >
                                  {e.status === "running" && <Loader2 className="w-2.5 h-2.5 animate-spin mr-1" />}
                                  {e.status}
                                </Badge>
                              </TableCell>
                              <TableCell className="text-xs text-slate-500">
                                {e.started_at ? new Date(e.started_at).toLocaleString() : "—"}
                              </TableCell>
                              <TableCell className="text-xs text-slate-500">
                                {e.completed_at ? new Date(e.completed_at).toLocaleString() : "—"}
                              </TableCell>
                              <TableCell>
                                <Button variant="ghost" size="sm" onClick={() => loadLogs(e.id)} className="h-8 text-xs">
                                  <List className="w-3.5 h-3.5 mr-1" /> Logs
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </Card>
                  )}
                </div>
              )}

            </div>
          )}
        </div>
      </main>

      {/* ========== LOG VIEWER MODAL ========== */}
      <Dialog open={!!viewingLogs} onOpenChange={(open) => !open && setViewingLogs(null)}>
        <DialogContent className="max-w-4xl p-0 overflow-hidden border-none shadow-2xl bg-slate-950 text-slate-100 flex flex-col h-[80vh]">
          <DialogTitle className="hidden">Execution Logs</DialogTitle>
          <div className="p-4 border-b border-slate-800 flex items-center justify-between bg-slate-900">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-blue-500/20 rounded-md">
                <Terminal className="w-5 h-5 text-blue-400" />
              </div>
              <div>
                <h3 className="text-sm font-bold text-white">Execution Terminal</h3>
                <p className="text-[10px] text-slate-400 font-mono">ID: {viewingLogs}</p>
              </div>
            </div>
            <Button variant="ghost" size="sm" onClick={() => setViewingLogs(null)} className="h-8 w-8 p-0 text-slate-400 hover:text-white hover:bg-slate-800">
              <X className="w-4 h-4" />
            </Button>
          </div>

          <div className="flex-1 overflow-y-auto p-4 font-mono text-[11px] space-y-1.5 scrollbar-thin scrollbar-thumb-slate-800">
            {loadingLogs ? (
              <div className="flex flex-col items-center justify-center h-full gap-3 py-20 text-slate-500">
                <Loader2 className="w-8 h-8 animate-spin" />
                <p>Loading real-time logs from agent...</p>
              </div>
            ) : executionLogs.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-full py-20 text-slate-600 italic">
                <p>No log entries found for this execution.</p>
              </div>
            ) : (
              (executionLogs || []).map((log: ExecutionLog, i: number) => (
                <div key={log.id || i} className="group flex gap-3 hover:bg-slate-900/50 py-0.5 px-1 -mx-1 rounded">
                  <span className="text-slate-600 shrink-0 select-none w-14">[{new Date(log.created_at).toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })}]</span>
                  <span className={`w-12 shrink-0 font-bold select-none ${
                    log.level === 'ERROR' ? 'text-rose-500' : 
                    log.level === 'WARN' ? 'text-amber-500' : 
                    'text-emerald-500'
                  }`}>
                    {log.level}
                  </span>
                  <span className="text-slate-300 break-all whitespace-pre-wrap">{log.message}</span>
                </div>
              ))
            )}
          </div>
          
          <div className="p-3 bg-slate-900 border-t border-slate-800 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Badge variant="outline" className="bg-slate-800 border-slate-700 text-[10px] font-mono lowercase">{executionLogs.length} events</Badge>
            </div>
            <p className="text-[10px] text-slate-500">Press ESC to close terminal</p>
          </div>
        </DialogContent>
      </Dialog>

      {/* ========== FULL SYSTEM BLUEPRINT MODAL ========== */}
      <Dialog open={!!viewingBlueprint} onOpenChange={(open) => !open && setViewingBlueprint(null)}>
        <DialogContent className="max-w-4xl p-0 overflow-hidden border-none shadow-2xl bg-white flex flex-col h-[85vh]">
          <DialogTitle className="hidden">System Blueprint</DialogTitle>
          <div className="p-6 border-b flex items-center justify-between bg-slate-50">
            <div className="flex items-center gap-4">
              <div className="p-3 bg-blue-600 rounded-xl shadow-lg shadow-blue-100">
                <Shield className="w-7 h-7 text-white" />
              </div>
              <div>
                <h3 className="text-2xl font-bold text-slate-900 leading-none">{viewingBlueprint?.alias}</h3>
                <p className="text-xs text-slate-500 mt-1.5 uppercase font-bold tracking-widest opacity-60">Full Architectural & Logic Blueprint</p>
              </div>
            </div>
            <Button variant="ghost" size="sm" onClick={() => setViewingBlueprint(null)} className="rounded-full h-10 w-10 p-0 hover:bg-slate-200">
              <X className="w-6 h-6" />
            </Button>
          </div>

          <div className="flex-1 overflow-y-auto p-10 bg-slate-50/30">
            <div className="max-w-3xl mx-auto space-y-10">
              
              {/* Architecture Visualization Section */}
              <section>
                <div className="flex items-center gap-2 mb-4">
                  <LayoutDashboard className="w-4 h-4 text-slate-400" />
                  <h4 className="text-[11px] font-bold text-slate-400 uppercase tracking-widest">Architecture Train</h4>
                </div>
                <div className="bg-white border rounded-2xl p-8 shadow-sm">
                   {viewingBlueprint?.steps && (
                    <div className="flex items-center justify-center gap-0">
                      {viewingBlueprint.steps.map((step: Step, i: number) => {
                        const Icon = (icons as any)[step.icon] || icons.Zap;
                        return (
                          <div key={step.id} className="flex items-center group">
                            <div className="relative flex flex-col items-center">
                              <div className="w-14 h-14 rounded-2xl bg-blue-50 border border-blue-100 flex items-center justify-center text-blue-600 shadow-sm group-hover:scale-110 group-hover:bg-blue-600 group-hover:text-white transition-all duration-300">
                                <Icon className="w-7 h-7" />
                              </div>
                              <span className="text-[10px] font-bold mt-2 text-slate-500 uppercase tracking-tighter">{step.label}</span>
                            </div>
                            {i < (viewingBlueprint.steps?.length || 0) - 1 && (
                              <div className="w-12 h-0.5 bg-slate-200 mt-[-20px]" />
                            )}
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              </section>

              {/* Logic Documentation Section */}
              <section>
                <div className="flex items-center gap-2 mb-4">
                  <FileCode2 className="w-4 h-4 text-slate-400" />
                  <h4 className="text-[11px] font-bold text-slate-400 uppercase tracking-widest">Logic Manifest</h4>
                </div>
                
                <div className="bg-white border rounded-2xl overflow-hidden shadow-sm flex flex-col">
                  {loadingLogic ? (
                    <div className="py-20 flex flex-col items-center gap-4 text-slate-400">
                      <Loader2 className="w-10 h-10 animate-spin" />
                      <p className="text-sm">Retrieving system logic...</p>
                    </div>
                  ) : (
                    <>
                      <div className="bg-blue-600 p-4 shrink-0 flex items-center justify-between">
                        <div className="flex items-center gap-2 text-white">
                          <Activity className="w-4 h-4" />
                          <span className="text-xs font-bold uppercase tracking-wider">AI Collaboration Bible</span>
                        </div>
                        <Button 
                          size="xs"
                          className="bg-white/20 hover:bg-white/30 text-white border-white/20 text-[10px] font-bold h-7"
                          onClick={() => {
                            navigator.clipboard.writeText(logicContent);
                            toast.success("Blueprint copied!");
                          }}
                        >
                          COPY FOR AI
                        </Button>
                      </div>
                      <div className="p-8 prose prose-slate max-w-none prose-sm leading-relaxed text-slate-700">
                        <div className="whitespace-pre-wrap font-sans text-base">
                          {logicContent}
                        </div>
                      </div>
                    </>
                  )}
                </div>
              </section>

              <div className="bg-amber-50 border border-amber-100 rounded-xl p-5 flex gap-4">
                <AlertCircle className="w-5 h-5 text-amber-500 shrink-0 mt-0.5" />
                <div>
                  <h5 className="text-sm font-bold text-amber-900">How to use this Blueprint</h5>
                  <p className="text-xs text-amber-800 leading-relaxed mt-1">
                    This document is a 1:1 map of the Go source code. If you want to change how this workflow works, copy the 
                    <strong> Logic Manifest</strong> above, paste it to your AI assistant, and describe your change. 
                    I will then implement it in code and update this blueprint automatically.
                  </p>
                </div>
              </div>
            </div>
          </div>
          
          <div className="p-5 bg-white border-t flex justify-end gap-3 shrink-0">
            <Button variant="outline" onClick={() => setViewingBlueprint(null)}>Close</Button>
            <Button className="bg-blue-600 hover:bg-blue-700 text-white" onClick={() => {
               navigator.clipboard.writeText(logicContent);
               toast.success("Blueprint copied!");
            }}>Copy Blueprint</Button>
          </div>
        </DialogContent>
      </Dialog>


      <style jsx global>{`
        @keyframes progress {
          0% { transform: translateX(-100%); }
          100% { transform: translateX(300%); }
        }
      `}</style>
      <Toaster position="top-right" expand={false} richColors />
    </div>
  );
}
