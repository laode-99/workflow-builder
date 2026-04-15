"use client";

import React, { useState, useEffect, useCallback } from "react";
import { 
  Search, Filter, RefreshCw, MoreHorizontal, User, 
  Phone, Calendar, AlertCircle, TrendingUp, CheckCircle2, XCircle, Clock
} from "lucide-react";
import { api, Lead, Business, ChatMessage } from "@/lib/api";
import { format } from "date-fns";
import { toast } from "sonner";
import ConversationViewer from "@/components/ConversationViewer";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";

export default function LeadsMonitoringPage() {
  const [businesses, setBusinesses] = useState<Business[]>([]);
  const [activeBiz, setActiveBiz] = useState<string>("");
  const [leads, setLeads] = useState<Lead[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);

  const [selectedLead, setSelectedLead] = useState<Lead | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [showViewer, setShowViewer] = useState(false);

  // Poll leads
  const fetchLeads = useCallback(async (bid: string, s: string, p: number) => {
    if (!bid) return;
    try {
      const res = await api.listLeads(bid, p, 50, s);
      setLeads(res.items);
      setTotal(res.total);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    api.listBusinesses().then(biz => {
      setBusinesses(biz);
      if (biz.length > 0) setActiveBiz(biz[0].id);
    });
  }, []);

  useEffect(() => {
    if (activeBiz) {
      setLoading(true);
      fetchLeads(activeBiz, search, page);
      const interval = setInterval(() => fetchLeads(activeBiz, search, page), 10000);
      return () => clearInterval(interval);
    }
  }, [activeBiz, search, page, fetchLeads]);

  const openViewer = async (lead: Lead) => {
    setSelectedLead(lead);
    setShowViewer(true);
    setLoadingMessages(true);
    try {
      const res = await api.listMessages(lead.id);
      setMessages(res);
    } catch (e) {
      toast.error("Failed to load conversation history");
    } finally {
      setLoadingMessages(false);
    }
  };

  const getInterestGlow = (interest: string) => {
    switch (interest?.toLowerCase()) {
      case "tertarik site visit (hot leads)": return "bg-green-500/10 text-green-400 border-green-500/20 shadow-[0_0_10px_rgba(34,197,94,0.15)]";
      case "tertarik untuk dihubungi (warm leads)": return "bg-blue-500/10 text-blue-400 border-blue-500/20 shadow-[0_0_10px_rgba(59,130,246,0.15)]";
      case "tidak mau atau tidak tertarik (cold leads)": return "bg-slate-500/10 text-slate-400 border-slate-500/20";
      default: return "bg-slate-800 text-slate-500 border-white/5";
    }
  };

  return (
    <div className="space-y-8 animate-in fade-in slide-in-from-bottom-4 duration-700">
      {/* Header Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        {[
          { label: "Total Leads", value: total, icon: User, color: "text-blue-400" },
          { label: "Hot Leads", value: leads.filter(l => l.interest?.includes("Hot")).length, icon: TrendingUp, color: "text-green-400" },
          { label: "SVS Scheduled", value: leads.filter(l => !!l.svs_date).length, icon: Calendar, color: "text-purple-400" },
          { label: "Pending Retries", value: leads.filter(l => l.attempt < 5 && !l.interest).length, icon: Clock, color: "text-amber-400" },
        ].map((stat, i) => (
          <div key={i} className="bg-white/[0.03] border border-white/5 rounded-2xl p-6 backdrop-blur-sm shadow-xl">
            <div className="flex items-center justify-between mb-4">
              <stat.icon className={`h-5 w-5 ${stat.color} opacity-80`} />
              <span className="text-[10px] uppercase tracking-wider text-slate-500 font-bold">Real-time</span>
            </div>
            <p className="text-2xl font-bold text-white mb-1">{stat.value}</p>
            <h3 className="text-xs text-slate-400 font-medium">{stat.label}</h3>
          </div>
        ))}
      </div>

      {/* Main Table Panel */}
      <div className="bg-white/[0.03] border border-white/5 rounded-3xl overflow-hidden shadow-2xl backdrop-blur-xl">
        {/* Toolbar */}
        <div className="p-6 border-b border-white/5 flex flex-col md:flex-row gap-4 justify-between items-center bg-white/[0.01]">
          <div className="flex items-center gap-4 w-full md:w-auto">
            <select 
              value={activeBiz} 
              onChange={(e) => setActiveBiz(e.target.value)}
              className="bg-[#0f172a] border border-white/10 rounded-xl px-4 py-2 text-sm text-slate-300 focus:outline-none focus:ring-2 focus:ring-blue-500/50"
            >
              {businesses.map(b => <option key={b.id} value={b.id}>{b.name}</option>)}
            </select>
            <div className="relative group w-full md:w-80">
              <Search className="absolute left-3 top-2.5 h-4 w-4 text-slate-500 transition-colors group-focus-within:text-blue-400" />
              <input 
                placeholder="Search phone, name, or status..."
                className="w-full bg-[#0f172a] border border-white/10 rounded-xl pl-10 pr-4 py-2 text-sm text-slate-300 placeholder:text-slate-600 focus:outline-none focus:ring-2 focus:ring-blue-500/50 transition-all"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
          </div>
          
          <div className="flex items-center gap-3">
            <button className="p-2.5 rounded-xl border border-white/10 text-slate-400 hover:text-white hover:bg-white/5 transition-all">
              <Filter className="h-4 w-4" />
            </button>
            <button 
              onClick={() => { setLoading(true); fetchLeads(activeBiz, search, page); }}
              className={`p-2.5 rounded-xl border border-white/10 text-slate-400 hover:text-white transition-all ${loading ? "animate-spin text-blue-400" : ""}`}
            >
              <RefreshCw className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Table Content */}
        <div className="overflow-x-auto">
          <table className="w-full text-left">
            <thead>
              <tr className="bg-white/[0.02] text-slate-500 text-[10px] uppercase font-bold tracking-widest">
                <th className="px-6 py-4">Lead Info</th>
                <th className="px-6 py-4">Status & Interest</th>
                <th className="px-6 py-4 text-center">Attempts</th>
                <th className="px-6 py-4">SVS Date</th>
                <th className="px-6 py-4">Last Update</th>
                <th className="px-6 py-4 text-center">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {leads.map((lead) => (
                <tr 
                  key={lead.id} 
                  onClick={() => openViewer(lead)}
                  className="hover:bg-white/[0.04] cursor-pointer transition-all group"
                >
                  <td className="px-6 py-5">
                    <div className="flex items-center gap-3">
                      <div className="h-10 w-10 rounded-xl bg-gradient-to-br from-blue-500/20 to-purple-500/20 flex items-center justify-center border border-white/5 group-hover:border-blue-500/30 transition-all">
                        <User className="h-4 w-4 text-blue-400" />
                      </div>
                      <div>
                        <p className="text-sm font-semibold text-white group-hover:text-blue-400 transition-colors uppercase tracking-tight">{lead.name || "Semua Orang"}</p>
                        <p className="text-[11px] text-slate-500 flex items-center gap-1 mt-0.5">
                          <Phone className="h-3 w-3" /> {lead.phone}
                        </p>
                      </div>
                    </div>
                  </td>
                  <td className="px-6 py-5">
                    <div className={`inline-flex px-3 py-1 rounded-full border text-[10px] font-bold uppercase tracking-tight ${getInterestGlow(lead.interest)}`}>
                      {lead.interest || "New Request"}
                    </div>
                    {lead.interest2 && (
                      <p className="text-[9px] text-slate-500 mt-1.5 italic">Chat: {lead.interest2}</p>
                    )}
                  </td>
                  <td className="px-6 py-5">
                    <div className="flex items-center justify-center gap-1">
                      {[1, 2, 3, 4, 5].map((step) => (
                        <div 
                          key={step}
                          className={`h-1.5 w-1.5 rounded-full ${
                            step <= lead.attempt 
                              ? "bg-blue-500 shadow-[0_0_8px_rgba(59,130,246,0.8)]" 
                              : "bg-slate-800"
                          }`}
                        />
                      ))}
                      <span className="text-xs text-slate-400 font-mono ml-2">{lead.attempt}/5</span>
                    </div>
                  </td>
                  <td className="px-6 py-5">
                    {lead.svs_date ? (
                      <div className="flex items-center gap-2 text-xs text-purple-400">
                        <Calendar className="h-3 w-3" />
                        <span className="font-semibold">{format(new Date(lead.svs_date), "dd MMM yyyy")}</span>
                      </div>
                    ) : (
                      <span className="text-xs text-slate-600">—</span>
                    )}
                  </td>
                  <td className="px-6 py-5">
                    <p className="text-[11px] text-slate-400">{format(new Date(lead.updated_at), "HH:mm, dd MMM")}</p>
                    <p className="text-[9px] text-slate-600 font-mono mt-1">v.{lead.version}</p>
                  </td>
                  <td className="px-6 py-5 text-center">
                    <button className="p-2 rounded-lg hover:bg-white/5 text-slate-500 hover:text-white transition-all">
                      <MoreHorizontal className="h-5 w-5" />
                    </button>
                  </td>
                </tr>
              ))}
              {leads.length === 0 && !loading && (
                <tr>
                  <td colSpan={6} className="px-6 py-20 text-center">
                    <AlertCircle className="h-10 w-10 text-slate-700 mx-auto mb-3" />
                    <p className="text-slate-500 text-sm">No leads found for this project filter.</p>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        
        {/* Pagination placeholder */}
        <div className="p-4 bg-white/[0.01] border-t border-white/5 flex justify-end">
          <div className="flex gap-2">
            <button className="px-3 py-1 rounded-lg border border-white/10 text-[11px] text-slate-500 hover:text-white transition-all disabled:opacity-30" disabled={page <= 1}>Prev</button>
            <button className="px-3 py-1 rounded-lg border border-white/10 text-[11px] text-slate-500 hover:text-white transition-all">Next</button>
          </div>
        </div>
      </div>

      {/* Conversation Viewer Modal */}
      <Dialog open={showViewer} onOpenChange={setShowViewer}>
        <DialogContent className="max-w-2xl bg-[#0f172a]/95 backdrop-blur-2xl border-white/10 p-0 overflow-hidden shadow-[0_0_50px_rgba(0,0,0,0.5)]">
          <DialogHeader className="p-6 border-b border-white/10 flex-row items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="h-12 w-12 rounded-2xl bg-gradient-to-br from-blue-500/20 to-purple-500/20 flex items-center justify-center border border-white/5 shadow-inner">
                <User className="h-5 w-5 text-blue-400" />
              </div>
              <div className="flex flex-col">
                <DialogTitle className="text-white font-bold tracking-tight uppercase">
                  {selectedLead?.name || "Semua Orang"}
                </DialogTitle>
                <span className="text-xs text-slate-500 font-mono tracking-tighter">
                  {selectedLead?.phone} • {selectedLead?.interest || "New Lead"}
                </span>
              </div>
            </div>
          </DialogHeader>

          <ConversationViewer 
            messages={messages} 
            loading={loadingMessages} 
            leadName={selectedLead?.name} 
          />

          <div className="p-4 bg-white/[0.02] border-t border-white/10 flex justify-between items-center">
            <p className="text-[10px] text-slate-500 uppercase font-bold tracking-widest">
              Live Transcript • Lina AI v.2
            </p>
            <button 
              onClick={() => openViewer(selectedLead!)} 
              className="text-xs text-blue-400 hover:text-white transition-colors flex items-center gap-2"
            >
              <RefreshCw className={`h-3 w-3 ${loadingMessages ? "animate-spin" : ""}`} />
              Refresh
            </button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
