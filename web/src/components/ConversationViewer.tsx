"use client";

import React from "react";
import { User, Zap, ShieldAlert, Bot } from "lucide-react";
import { ChatMessage } from "@/lib/api";
import { format } from "date-fns";

interface Props {
  messages: ChatMessage[];
  leadName?: string;
  loading?: boolean;
}

export default function ConversationViewer({ messages, leadName, loading }: Props) {
  if (loading) {
    return (
      <div className="flex flex-col gap-4 p-6 animate-pulse">
        {[1, 2, 3].map((i) => (
          <div key={i} className={`flex gap-3 ${i % 2 === 0 ? "flex-row-reverse" : ""}`}>
            <div className="h-8 w-8 rounded-full bg-white/5" />
            <div className="h-16 w-48 rounded-2xl bg-white/5" />
          </div>
        ))}
      </div>
    );
  }

  if (messages.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-slate-500">
        <Bot className="h-10 w-10 mb-3 opacity-20" />
        <p className="text-sm">No messages found for this lead yet.</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 p-6 overflow-y-auto max-h-[600px] scrollbar-thin scrollbar-thumb-white/10 scrollbar-track-transparent">
      {messages.map((msg, i) => {
        const isAI = msg.role === "assistant";
        const isSystem = msg.role === "system";

        if (isSystem) {
          return (
            <div key={msg.id} className="flex justify-center">
              <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-white/[0.03] border border-white/5 text-[10px] text-slate-500 uppercase font-bold tracking-widest">
                <ShieldAlert className="h-3 w-3" />
                {msg.content}
              </div>
            </div>
          );
        }

        return (
          <div 
            key={msg.id} 
            className={`flex gap-3 max-w-[85%] ${isAI ? "self-start" : "self-end flex-row-reverse items-end"}`}
          >
            <div className={`h-8 w-8 rounded-xl flex items-center justify-center shrink-0 border border-white/5 ${
              isAI 
                ? "bg-blue-600/20 text-blue-400 shadow-[0_0_10px_rgba(37,99,235,0.2)]" 
                : "bg-purple-600/20 text-purple-400 shadow-[0_0_10px_rgba(147,51,234,0.2)]"
            }`}>
              {isAI ? <Zap className="h-4 w-4" /> : <User className="h-4 w-4" />}
            </div>

            <div className="flex flex-col gap-1">
              <div className={`p-4 rounded-3xl text-sm leading-relaxed whitespace-pre-wrap ${
                isAI 
                  ? "bg-white/[0.04] border border-white/5 text-slate-200" 
                  : "bg-blue-600/20 border border-blue-500/20 text-white"
              }`}>
                {msg.content}
              </div>
              <span className={`text-[10px] text-slate-500 flex items-center gap-1 mt-1 ${isAI ? "self-start" : "self-end"}`}>
                {msg.role === "assistant" ? "Lina AI" : (leadName || "Lead")} • {format(new Date(msg.created_at), "HH:mm")}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}
