"use client";

import React, { useState, useEffect, useCallback } from "react";
import { 
  Plus, Save, History, Zap, MessageSquare, 
  Settings, Search, Shield, ChevronRight, FileText, CheckCircle2, RotateCcw, XCircle
} from "lucide-react";
import { api, ProjectPrompt, Business } from "@/lib/api";
import { format } from "date-fns";
import { toast } from "sonner";

export default function PromptStudioPage() {
  const [businesses, setBusinesses] = useState<Business[]>([]);
  const [activeBiz, setActiveBiz] = useState<string>("");
  const [prompts, setPrompts] = useState<ProjectPrompt[]>([]);
  const [loading, setLoading] = useState(true);
  
  const [selectedKind, setSelectedKind] = useState<string>("chatbot_system");
  const [editorContent, setEditorContent] = useState("");
  const [saving, setSaving] = useState(false);

  const promptKinds = [
    { id: "chatbot_system", label: "Lina AI Core", icon: Zap, desc: "The main personality and instructions for the chatbot." },
    { id: "chatbot_faq", label: "FAQ Reference", icon: FileText, desc: "Manual knowledge base injection for specific questions." },
    { id: "intent_classifier", label: "Intent Classifier", icon: Shield, desc: "Rules for tagging leads as Callback, Cold, or Agent." },
    { id: "spam_classifier", label: "Spam Guard", icon: XCircle, desc: "Rules for identifying bot/spam traffic." },
  ];

  const fetchPrompts = useCallback(async (bid: string) => {
    if (!bid) return;
    try {
      const res = await api.listPrompts(bid);
      setPrompts(res);
      // Default editor to the active prompt of selected kind
      const active = res.find(p => p.kind === selectedKind && p.is_active);
      if (active) setEditorContent(active.content);
      else setEditorContent("");
    } catch (e) {
      toast.error("Failed to load prompts");
    } finally {
      setLoading(false);
    }
  }, [selectedKind]);

  useEffect(() => {
    api.listBusinesses().then(biz => {
      setBusinesses(biz);
      if (biz.length > 0) setActiveBiz(biz[0].id);
    });
  }, []);

  useEffect(() => {
    if (activeBiz) {
      setLoading(true);
      fetchPrompts(activeBiz);
    }
  }, [activeBiz, fetchPrompts]);

  const savePrompt = async () => {
    if (!activeBiz || !editorContent) return;
    setSaving(true);
    try {
      await api.createPrompt(activeBiz, {
        kind: selectedKind,
        content: editorContent,
      });
      toast.success("Prompt saved as new active version");
      fetchPrompts(activeBiz);
    } catch (e) {
      toast.error("Failed to save prompt");
    } finally {
      setSaving(false);
    }
  };

  const history = prompts.filter(p => p.kind === selectedKind).sort((a, b) => b.version - a.version);

  return (
    <div className="h-full flex flex-col gap-6 animate-in fade-in slide-in-from-bottom-4 duration-700">
      {/* Header */}
      <div className="flex justify-between items-center bg-white/[0.03] border border-white/5 rounded-2xl p-6 backdrop-blur-sm">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-3">
            <Zap className="h-6 w-6 text-blue-400" />
            AI Prompt Studio
          </h2>
          <p className="text-xs text-slate-400 mt-1">Manage the &quot;brain&quot; of each project. Versioned, safe, and real-time updates.</p>
        </div>
        
        <select 
          value={activeBiz} 
          onChange={(e) => setActiveBiz(e.target.value)}
          className="bg-[#0f172a] border border-white/10 rounded-xl px-4 py-2.5 text-sm text-slate-300 font-semibold focus:outline-none focus:ring-2 focus:ring-blue-500/50"
        >
          {businesses.map(b => <option key={b.id} value={b.id}>{b.name}</option>)}
        </select>
      </div>

      <div className="flex-1 grid grid-cols-12 gap-6 min-h-0">
        {/* Kinds List (Sidebar) */}
        <div className="col-span-12 md:col-span-3 flex flex-col gap-3 overflow-y-auto">
          {promptKinds.map((kind) => {
            const isActive = selectedKind === kind.id;
            const current = prompts.find(p => p.kind === kind.id && p.is_active);
            return (
              <button
                key={kind.id}
                onClick={() => setSelectedKind(kind.id)}
                className={`flex flex-col text-left p-4 rounded-2xl border transition-all ${
                  isActive 
                    ? "bg-blue-600/10 border-blue-500/30 ring-1 ring-blue-500/20" 
                    : "bg-white/[0.02] border-white/5 hover:bg-white/[0.05]"
                }`}
              >
                <div className="flex items-center justify-between mb-2">
                  <kind.icon className={`h-4 w-4 ${isActive ? "text-blue-400" : "text-slate-500"}`} />
                  {current && <span className="text-[9px] font-bold text-blue-400 opacity-80 uppercase tracking-widest">v.{current.version}</span>}
                </div>
                <h3 className={`text-sm font-bold ${isActive ? "text-white" : "text-slate-400"}`}>{kind.label}</h3>
                <p className="text-[10px] text-slate-500 mt-1 leading-relaxed">{kind.desc}</p>
              </button>
            );
          })}
        </div>

        {/* Editor Area */}
        <div className="col-span-12 md:col-span-6 flex flex-col bg-white/[0.03] border border-white/5 rounded-3xl overflow-hidden shadow-2xl backdrop-blur-xl">
          <div className="p-4 bg-white/[0.02] border-b border-white/5 flex justify-between items-center">
            <div className="flex items-center gap-2">
              <span className="text-[10px] uppercase font-bold tracking-widest text-slate-500">Editor</span>
              <ChevronRight className="h-3 w-3 text-slate-700" />
              <span className="text-xs font-semibold text-blue-400">{selectedKind}</span>
            </div>
            <button 
              onClick={savePrompt}
              disabled={saving || !editorContent}
              className="flex items-center gap-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white px-4 py-1.5 rounded-xl text-xs font-bold transition-all shadow-[0_0_15px_rgba(37,99,235,0.3)]"
            >
              {saving ? <RotateCcw className="h-3 w-3 animate-spin" /> : <Save className="h-3 w-3" />}
              SAVE VERSION
            </button>
          </div>
          <div className="flex-1 relative">
            <textarea 
              className="w-full h-full bg-transparent p-6 text-sm text-slate-300 font-mono leading-relaxed resize-none focus:outline-none"
              value={editorContent}
              onChange={(e) => setEditorContent(e.target.value)}
              placeholder="Paste your system prompt instructions here..."
            />
          </div>
        </div>

        {/* History Area */}
        <div className="col-span-12 md:col-span-3 flex flex-col bg-white/[0.03] border border-white/5 rounded-3xl overflow-hidden shadow-xl backdrop-blur-sm">
          <div className="p-4 bg-white/[0.01] border-b border-white/5">
            <h3 className="text-[10px] uppercase font-bold tracking-widest text-slate-500 flex items-center gap-2">
              <History className="h-3 w-3" />
              Change History
            </h3>
          </div>
          <div className="flex-1 overflow-y-auto divide-y divide-white/5">
            {history.map((h) => (
              <button 
                key={h.id}
                onClick={() => setEditorContent(h.content)}
                className="w-full text-left p-4 hover:bg-white/[0.02] transition-all group"
              >
                <div className="flex items-center justify-between mb-1">
                  <span className={`text-[10px] font-bold ${h.is_active ? "text-blue-400" : "text-slate-500"}`}>
                    VERSION {h.version} {h.is_active && "• ACTIVE"}
                  </span>
                  <span className="text-[9px] text-slate-700 whitespace-nowrap">
                    {format(new Date(h.created_at), "dd MMM HH:mm")}
                  </span>
                </div>
                <p className="text-[11px] text-slate-400 truncate opacity-60 group-hover:opacity-100 transition-opacity whitespace-pre-wrap line-clamp-2">
                  {h.content}
                </p>
              </button>
            ))}
            {history.length === 0 && (
              <div className="p-8 text-center">
                <p className="text-xs text-slate-600">No versions yet.</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
