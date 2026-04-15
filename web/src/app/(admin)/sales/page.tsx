"use client";

import React, { useState, useEffect, useCallback } from "react";
import { 
  Users, Plus, Trash2, Power, User, Shield, 
  Search, ShieldCheck, Mail, Phone, MoreVertical, AlertCircle
} from "lucide-react";
import { api, SalesAssignment, Business } from "@/lib/api";
import { toast } from "sonner";

export default function SalesRosterPage() {
  const [businesses, setBusinesses] = useState<Business[]>([]);
  const [activeBiz, setActiveBiz] = useState<string>("");
  const [roster, setRoster] = useState<SalesAssignment[]>([]);
  const [loading, setLoading] = useState(true);

  const [newName, setNewName] = useState("");
  const [newSpv, setNewSpv] = useState("");
  const [isAdding, setIsAdding] = useState(false);

  const fetchRoster = useCallback(async (bid: string) => {
    if (!bid) return;
    try {
      const res = await api.listSales(bid);
      setRoster(res);
    } catch (e) {
      toast.error("Failed to load sales roster");
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
      fetchRoster(activeBiz);
    }
  }, [activeBiz, fetchRoster]);

  const addSales = async () => {
    if (!newName.trim() || !activeBiz) return;
    try {
      await api.upsertSales(activeBiz, {
        sales_name: newName,
        spv_name: newSpv,
        is_active: true,
      });
      toast.success("Sales added to roster");
      setNewName(""); setNewSpv(""); setIsAdding(false);
      fetchRoster(activeBiz);
    } catch (e) {
      toast.error("Failed to add sales");
    }
  };

  const toggleActive = async (id: string) => {
    try {
      await api.toggleSales(id);
      fetchRoster(activeBiz);
      toast.success("Roster status updated");
    } catch (e) {
      toast.error("Update failed");
    }
  };

  return (
    <div className="space-y-8 animate-in fade-in slide-in-from-bottom-4 duration-700">
      <div className="flex justify-between items-center bg-white/[0.03] border border-white/5 rounded-2xl p-6 backdrop-blur-sm">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-3">
            <ShieldCheck className="h-6 w-6 text-green-400" />
            Sales Roster
          </h2>
          <p className="text-xs text-slate-400 mt-1">Manage sales assignment groups. Leads are automatically assigned via Round-Robin.</p>
        </div>
        
        <select 
          value={activeBiz} 
          onChange={(e) => setActiveBiz(e.target.value)}
          className="bg-[#0f172a] border border-white/10 rounded-xl px-4 py-2.5 text-sm text-slate-300 font-semibold focus:outline-none focus:ring-2 focus:ring-blue-500/50"
        >
          {businesses.map(b => <option key={b.id} value={b.id}>{b.name}</option>)}
        </select>
      </div>

      <div className="grid grid-cols-12 gap-8">
        {/* Statistics */}
        <div className="col-span-12 md:col-span-4 flex flex-col gap-6">
          <div className="bg-white/[0.03] border border-white/5 rounded-2xl p-6">
            <h3 className="text-xs uppercase font-bold tracking-widest text-slate-500 mb-4">Current Roster</h3>
            <div className="flex items-center justify-between">
              <div className="text-center px-4 border-r border-white/5 flex-1">
                <p className="text-2xl font-bold text-white">{roster.length}</p>
                <p className="text-[10px] text-slate-500">Total Sales</p>
              </div>
              <div className="text-center px-4 flex-1">
                <p className="text-2xl font-bold text-green-400">{roster.filter(r => r.is_active).length}</p>
                <p className="text-[10px] text-slate-500">Online</p>
              </div>
            </div>
          </div>

          <div className="bg-white/[0.03] border border-white/5 rounded-2xl p-6">
            <h3 className="text-xs uppercase font-bold tracking-widest text-slate-500 mb-4">Active Supervisors</h3>
            <div className="flex flex-wrap gap-2">
              {Array.from(new Set(roster.map(r => r.spv_name).filter(Boolean))).map((spv, i) => (
                <span key={i} className="px-2 py-1 rounded-lg bg-blue-500/10 border border-blue-500/20 text-[10px] text-blue-400 font-bold uppercase">
                  SPV: {spv}
                </span>
              ))}
            </div>
          </div>
        </div>

        {/* List & Adder */}
        <div className="col-span-12 md:col-span-8 flex flex-col gap-6">
          <div className="bg-white/[0.03] border border-white/5 rounded-3xl overflow-hidden shadow-2xl backdrop-blur-xl">
            <div className="p-6 border-b border-white/5 flex justify-between items-center bg-white/[0.01]">
              <h3 className="text-sm font-bold text-white tracking-tight">Assignment Pool</h3>
              <button 
                onClick={() => setIsAdding(!isAdding)}
                className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-1.5 rounded-xl text-xs font-bold transition-all shadow-lg"
              >
                {isAdding ? "CANCEL" : "ADD SALES"}
              </button>
            </div>

            {isAdding && (
              <div className="p-6 bg-blue-600/5 border-b border-white/5 flex flex-wrap gap-4 items-end animate-in fade-in slide-in-from-top-2">
                <div className="flex-1 min-w-[200px]">
                  <label className="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-1.5 block">Full Name</label>
                  <input 
                    className="w-full bg-[#0f172a] border border-white/10 rounded-xl px-4 py-2 text-sm text-white"
                    placeholder="e.g. Alex Mario"
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                  />
                </div>
                <div className="flex-1 min-w-[200px]">
                  <label className="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-1.5 block">Supervisor (Optional)</label>
                  <input 
                    className="w-full bg-[#0f172a] border border-white/10 rounded-xl px-4 py-2 text-sm text-white"
                    placeholder="e.g. SPV Utama"
                    value={newSpv}
                    onChange={(e) => setNewSpv(e.target.value)}
                  />
                </div>
                <button 
                  onClick={addSales}
                  disabled={!newName.trim()}
                  className="bg-green-600 hover:bg-green-700 disabled:opacity-50 text-white h-[38px] px-6 rounded-xl text-xs font-bold shadow-lg"
                >
                  SAVE
                </button>
              </div>
            )}

            <div className="divide-y divide-white/5">
              {roster.map((s) => (
                <div key={s.id} className="p-6 flex items-center justify-between hover:bg-white/[0.02] transition-all group">
                  <div className="flex items-center gap-4">
                    <div className="h-12 w-12 rounded-2xl bg-gradient-to-br from-blue-500/10 to-transparent flex items-center justify-center border border-white/5 text-blue-400 shadow-inner group-hover:border-blue-500/20 transition-all">
                      <User className="h-5 w-5" />
                    </div>
                    <div>
                      <h4 className="text-sm font-bold text-white uppercase tracking-tight">{s.sales_name}</h4>
                      {s.spv_name && (
                        <p className="text-[10px] text-slate-500 flex items-center gap-1 mt-0.5">
                          <Shield className="h-3 w-3" /> Reporting to: <span className="text-blue-400/70 font-semibold uppercase">{s.spv_name}</span>
                        </p>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center gap-6">
                    <button 
                      onClick={() => toggleActive(s.id)}
                      className={`flex items-center gap-2 px-3 py-1.5 rounded-xl border text-[10px] font-bold uppercase transition-all ${
                        s.is_active 
                          ? "bg-green-500/10 border-green-500/20 text-green-400" 
                          : "bg-slate-800 border-white/5 text-slate-500"
                      }`}
                    >
                      <Power className="h-3 w-3" />
                      {s.is_active ? "Online" : "Paused"}
                    </button>
                    <button className="text-slate-700 hover:text-red-400 transition-colors p-2 rounded-lg hover:bg-white/5">
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              ))}
              {roster.length === 0 && (
                <div className="p-16 text-center">
                  <AlertCircle className="h-10 w-10 text-slate-700 mx-auto mb-3 opacity-20" />
                  <p className="text-slate-500 text-sm">No sales assignment found in roster.</p>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
