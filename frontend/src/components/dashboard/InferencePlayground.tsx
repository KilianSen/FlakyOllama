import React, { useState } from 'react';
import { Terminal, Play, RefreshCw, Globe } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { toast } from 'sonner';
import sdk from '../../api';
import type { ClusterStatus, InferenceResponse } from '../../api';

interface InferencePlaygroundProps {
  status: ClusterStatus;
}

export const InferencePlayground: React.FC<InferencePlaygroundProps> = ({ status }) => {
  const [testResult, setTestResult] = useState<InferenceResponse | null>(null);
  const [testLoading, setTestLoading] = useState(false);

  const handleTest = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const model = fd.get('model') as string;
    const prompt = fd.get('prompt') as string;
    const node_id = fd.get('node_id') as string;

    if (model && prompt) {
      setTestLoading(true);
      const tid = toast.loading('Transmitting inference...');
      try {
        const res = await sdk.testInference({
          model,
          prompt,
          node_id: node_id !== "dynamic" ? node_id : undefined
        });
        setTestResult(res);
        toast.success(`Served by ${res.agent_id}`, { id: tid });
      } catch (err: any) {
        toast.error(err.message || 'Transmission error', { id: tid });
      } finally {
        setTestLoading(false);
      }
    }
  };

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
      <Card className="lg:col-span-2 border-none shadow-sm bg-background overflow-hidden flex flex-col">
        <CardHeader className="py-4 border-b bg-muted/10">
          <CardTitle className="text-xs font-black uppercase tracking-widest flex items-center gap-2">
            <Terminal size={16} className="text-primary" /> Inference Playground
          </CardTitle>
        </CardHeader>
        <CardContent className="p-6">
          <form onSubmit={handleTest} className="space-y-6">
            <div className="grid grid-cols-2 gap-6">
              <div className="space-y-2">
                <label className="text-[9px] font-black uppercase text-muted-foreground px-1 tracking-widest">Neural architecture</label>
                <Select name="model" required defaultValue={(status.all_models || [])[0]}>
                  <SelectTrigger className="h-10 font-bold text-xs border-2">
                    <SelectValue placeholder="Target" />
                  </SelectTrigger>
                  <SelectContent className="font-bold text-xs">
                    {(status.all_models || []).map(m => <SelectItem key={m} value={m}>{m.toUpperCase()}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <label className="text-[9px] font-black uppercase text-muted-foreground px-1 tracking-widest">Compute provider</label>
                <Select name="node_id" defaultValue="dynamic">
                  <SelectTrigger className="h-10 font-bold text-xs border-2">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent className="font-bold text-xs">
                    <SelectItem value="dynamic">DYNAMIC BALANCING</SelectItem>
                    {Object.values(status.nodes).map(n => <SelectItem key={n.id} value={n.id}>{n.id.toUpperCase()}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <Textarea name="prompt" placeholder="TRANSMIT PROMPT..." className="min-h-[120px] text-xs font-bold uppercase p-4 border-2 resize-none focus-visible:ring-primary/20" required />
            <div className="flex justify-end">
              <Button type="submit" disabled={testLoading} className="h-12 px-10 font-black text-xs tracking-[0.3em] uppercase shadow-xl shadow-primary/20 bg-primary">
                {testLoading ? <RefreshCw className="animate-spin mr-3" size={16} /> : <Play className="mr-3" size={16} fill="currentColor" fillOpacity={0.2} />}
                Execute Inference
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <div className="space-y-8 flex flex-col">
        <Card className="border-none shadow-sm bg-background overflow-hidden flex flex-col h-full">
          <CardHeader className="py-4 border-b">
            <CardTitle className="text-xs font-black uppercase tracking-widest">Inference Response Buffer</CardTitle>
          </CardHeader>
          <CardContent className="p-0 flex-1 relative min-h-[300px]">
            <ScrollArea className="h-full bg-slate-950 p-6 absolute inset-0">
              <div className="text-slate-300 text-xs font-mono whitespace-pre-wrap leading-relaxed">
                {testResult ? (
                  <div>
                    <div className="border-b border-white/10 pb-4 mb-4 flex items-center justify-between">
                      <Badge variant="outline" className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20 text-[9px] font-black uppercase tracking-widest">Provider: {testResult.agent_id}</Badge>
                      <Button variant="ghost" size="sm" onClick={() => setTestResult(null)} className="h-6 text-[8px] font-black uppercase text-slate-500">Clear Buffer</Button>
                    </div>
                    {testResult.response}
                  </div>
                ) : (
                  <div className="h-full flex flex-col items-center justify-center pt-20 opacity-20 grayscale select-none">
                    <Globe className="animate-pulse mb-4" size={48} />
                    <span className="text-[10px] font-black uppercase tracking-[0.5em]">Awaiting Transmission</span>
                  </div>
                )}
              </div>
            </ScrollArea>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};
