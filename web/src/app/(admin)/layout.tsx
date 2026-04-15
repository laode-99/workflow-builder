"use client";

import React, { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { 
  Users, MessageSquare, ShieldCheck, Settings, LayoutDashboard, 
  BarChart3, ChevronLeft, ChevronRight, Zap, Bell
} from "lucide-react";

export default function LeadflowAdminLayout({ children }: { children: React.ReactNode }) {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const pathname = usePathname();

  const navItems = [
    { label: "Overview", icon: LayoutDashboard, href: "/admin" },
    { label: "Leads Monitoring", icon: Users, href: "/admin/leads" },
    { label: "AI Prompt Studio", icon: MessageSquare, href: "/admin/prompts" },
    { label: "Sales Roster", icon: ShieldCheck, href: "/admin/sales" },
    { label: "Analytics", icon: BarChart3, href: "/admin/analytics" },
  ];

  return (
    <div className="flex h-screen bg-[#020617] text-slate-200 overflow-hidden font-sans">
      {/* Sidebar */}
      <aside 
        className={`relative flex flex-col border-r border-white/5 bg-white/5 backdrop-blur-xl transition-all duration-300 ease-in-out ${
          isCollapsed ? "w-20" : "w-64"
        }`}
      >
        {/* Brand */}
        <div className="flex h-16 items-center px-6 border-b border-white/5">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-600 shadow-[0_0_15px_rgba(37,99,235,0.4)]">
              <Zap className="h-4 w-4 text-white" />
            </div>
            {!isCollapsed && (
              <span className="text-lg font-bold tracking-tight bg-gradient-to-r from-white to-slate-400 bg-clip-text text-transparent">
                Leadflow
              </span>
            )}
          </div>
        </div>

        {/* Navigation */}
        <nav className="flex-1 space-y-1 p-4 overflow-y-auto">
          {navItems.map((item) => {
            const isActive = pathname === item.href;
            return (
              <Link 
                key={item.href}
                href={item.href}
                className={`flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all group ${
                  isActive 
                    ? "bg-blue-600/10 text-blue-400 border border-blue-500/20" 
                    : "text-slate-400 hover:bg-white/5 hover:text-slate-100"
                }`}
              >
                <item.icon className={`h-5 w-5 shrink-0 ${isActive ? "text-blue-400" : "group-hover:text-blue-300"}`} />
                {!isCollapsed && <span className="text-sm font-medium">{item.label}</span>}
              </Link>
            );
          })}
        </nav>

        {/* Footer */}
        <div className="p-4 border-t border-white/5 space-y-4">
          <div className="px-3 py-3 rounded-xl bg-blue-500/5 border border-blue-500/10">
            <p className="text-[10px] font-bold text-blue-400 uppercase tracking-widest mb-1.5 flex items-center gap-1.5">
              <ShieldCheck size={10} /> System Policy
            </p>
            <p className="text-[10px] text-slate-500 leading-relaxed italic">
              Operational data is automatically archived every 6 months to ensure peak performance.
            </p>
          </div>
          
          <button className="flex items-center gap-3 px-3 py-2 text-slate-400 hover:text-white transition-colors w-full">
            <Settings className="h-5 w-5" />
            {!isCollapsed && <span className="text-sm font-medium">Settings</span>}
          </button>
        </div>

        {/* Collapse Toggle */}
        <button 
          onClick={() => setIsCollapsed(!isCollapsed)}
          className="absolute -right-3 top-20 flex h-6 w-6 items-center justify-center rounded-full border border-white/10 bg-[#0f172a] hover:bg-[#1e293b] text-slate-400"
        >
          {isCollapsed ? <ChevronRight size={14} /> : <ChevronLeft size={14} />}
        </button>
      </aside>

      {/* Main Content */}
      <main className="flex-1 flex flex-col min-w-0 h-full relative">
        {/* Topbar */}
        <header className="h-16 flex items-center justify-between px-8 border-b border-white/5 bg-white/[0.02] backdrop-blur-sm shrink-0">
          <div className="flex items-center gap-4">
            <h1 className="text-sm font-medium text-slate-400">
              {navItems.find(i => i.href === pathname)?.label || "Dashboard"}
            </h1>
          </div>
          
          <div className="flex items-center gap-6">
            <button className="relative text-slate-400 hover:text-white transition-colors">
              <Bell className="h-5 w-5" />
              <span className="absolute -top-1 -right-1 flex h-2 w-2 rounded-full bg-blue-500 shadow-[0_0_8px_rgba(59,130,246,0.6)]" />
            </button>
            <div className="flex items-center gap-3 pl-6 border-l border-white/10">
              <div className="flex flex-col items-end mr-3">
                <span className="text-xs font-semibold text-white">Admin Console</span>
                <span className="text-[10px] text-slate-500">Root Access</span>
              </div>
              <div className="h-9 w-9 rounded-xl bg-gradient-to-br from-blue-500 to-purple-600 p-[1px]">
                <div className="h-full w-full rounded-[10px] bg-[#0f172a] flex items-center justify-center font-bold text-xs">AD</div>
              </div>
            </div>
          </div>
        </header>

        {/* Dynamic Content */}
        <div className="flex-1 overflow-auto p-8 scrollbar-thin scrollbar-thumb-white/10 scrollbar-track-transparent">
          {children}
        </div>
      </main>
    </div>
  );
}
