"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { LayoutDashboard, Lock, Mail, Loader2, Cpu } from "lucide-react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent } from "@/components/ui/card";
import { toast } from "sonner";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      const { token, user } = await api.login(email, password);
      localStorage.setItem("fb_token", token);
      localStorage.setItem("fb_user", JSON.stringify(user));
      toast.success(`Welcome back, ${user.name}!`);
      router.push("/");
    } catch (err: any) {
      toast.error(err.message || "Invalid credentials");
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="min-h-screen bg-slate-50 flex items-center justify-center p-6 bg-[radial-gradient(circle_at_top_right,_var(--tw-gradient-stops))] from-blue-50/50 via-white to-slate-50">
      <div className="w-full max-w-[440px] space-y-8 animate-in fade-in zoom-in duration-500">
        <div className="flex flex-col items-center text-center space-y-4">
          <div className="w-16 h-16 rounded-3xl bg-blue-600 shadow-xl shadow-blue-200 flex items-center justify-center rotate-3 hover:rotate-0 transition-transform duration-500">
            <Cpu className="w-8 h-8 text-white" />
          </div>
          <div className="space-y-1">
            <h1 className="text-3xl font-black text-slate-900 tracking-tight">FlowBuilder</h1>
            <p className="text-slate-500 font-medium">Enterprise Workflow Engine</p>
          </div>
        </div>

        <Card className="border-0 shadow-2xl shadow-blue-100/50 rounded-[32px] overflow-hidden bg-white/80 backdrop-blur-xl border-t border-white">
          <CardContent className="p-10">
            <form onSubmit={handleLogin} className="space-y-6">
              <div className="space-y-2">
                <label className="text-[11px] font-black text-slate-400 uppercase tracking-[0.2em] ml-1">Email Address</label>
                <div className="relative group">
                  <Mail className="absolute left-4 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-300 group-focus-within:text-blue-500 transition-colors" />
                  <Input 
                    type="email" 
                    placeholder="name@company.com" 
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    className="h-14 pl-12 rounded-2xl border-slate-100 bg-slate-50/50 focus:bg-white transition-all text-slate-900 font-medium text-base"
                    required
                  />
                </div>
              </div>

              <div className="space-y-2">
                <label className="text-[11px] font-black text-slate-400 uppercase tracking-[0.2em] ml-1">Password</label>
                <div className="relative group">
                  <Lock className="absolute left-4 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-300 group-focus-within:text-blue-500 transition-colors" />
                  <Input 
                    type="password" 
                    placeholder="••••••••" 
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="h-14 pl-12 rounded-2xl border-slate-100 bg-slate-50/50 focus:bg-white transition-all text-slate-900 font-medium text-base"
                    required
                  />
                </div>
              </div>

              <Button 
                type="submit" 
                className="w-full h-14 bg-blue-600 hover:bg-blue-700 text-white rounded-2xl font-bold text-base shadow-lg shadow-blue-200 transition-all active:scale-[0.98] disabled:opacity-70"
                disabled={loading}
              >
                {loading ? <Loader2 className="w-5 h-5 animate-spin mx-auto" /> : "Sign In to Dashboard"}
              </Button>
            </form>

            <div className="mt-10 pt-8 border-t border-slate-50 text-center">
              <p className="text-xs text-slate-400 font-medium">
                Protected by Enterprise Audit Logging
              </p>
            </div>
          </CardContent>
        </Card>

        <p className="text-center text-slate-400 text-xs font-medium">
          &copy; 2026 FlowBuilder Suite v2.0
        </p>
      </div>
    </main>
  );
}
