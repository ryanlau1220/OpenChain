import React, { useEffect, useState } from 'react';
import { Bell, Radio, Shield, CheckCircle2 } from 'lucide-react';

export const WatchlistPanel: React.FC = () => {
  const [messages, setMessages] = useState<string[]>([]);
  const [wsConnected, setWsConnected] = useState(false);

  useEffect(() => {
    const wsUrl = (import.meta.env.VITE_API_URL || 'http://localhost:8081').replace(/^http/, 'ws') + '/ws';
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      setWsConnected(true);
      ws.send(JSON.stringify({ type: 'ping' }));
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        setMessages((prev) => [JSON.stringify(data), ...prev.slice(0, 10)]);
      } catch (err) {
        console.error(err);
      }
    };

    ws.onclose = () => setWsConnected(false);

    return () => ws.close();
  }, []);

  return (
    <div className="p-8 max-w-5xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-bold text-white font-sans flex items-center gap-2">
            <Bell className="w-5 h-5 text-cyan-400" />
            Continuous Wallet Monitoring & Real-time Stream
          </h2>
          <p className="text-xs text-slate-400">WebSocket feed powered by coder/websocket.</p>
        </div>

        <div className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-semibold ${
          wsConnected ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-red-500/10 text-red-400 border border-red-500/20'
        }`}>
          <Radio className={`w-3.5 h-3.5 ${wsConnected ? 'animate-pulse' : ''}`} />
          <span>{wsConnected ? 'WebSocket Live' : 'Disconnected'}</span>
        </div>
      </div>

      <div className="p-6 glass-panel rounded-2xl space-y-4">
        <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400">Live Transaction & Activity Feed</h3>
        <div className="bg-slate-950 p-4 rounded-xl font-mono text-xs text-slate-300 space-y-2 border border-slate-900 max-h-96 overflow-y-auto">
          {messages.length === 0 ? (
            <p className="text-slate-600 italic">Listening for real-time WebSocket events...</p>
          ) : (
            messages.map((m, idx) => (
              <div key={idx} className="py-1 border-b border-slate-900 last:border-0 flex items-center gap-2">
                <span className="text-cyan-400">[{new Date().toLocaleTimeString()}]</span>
                <span className="text-slate-200">{m}</span>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
