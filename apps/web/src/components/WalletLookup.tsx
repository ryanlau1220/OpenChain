import React from 'react';
import { AddressSummary, RiskEvaluation, LabelItem } from '../services/api';
import { Wallet, ShieldAlert, ArrowUpRight, ArrowDownLeft, Tag, Clock, ExternalLink } from 'lucide-react';

interface WalletLookupProps {
  summary: AddressSummary | null;
  risk: RiskEvaluation | null;
  labels: LabelItem[];
  loading: boolean;
  onTraceAddress: (addr: string) => void;
}

export const WalletLookup: React.FC<WalletLookupProps> = ({ summary, risk, labels, loading, onTraceAddress }) => {
  if (loading) {
    return (
      <div className="p-12 flex flex-col items-center justify-center text-slate-400 gap-3">
        <div className="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin" />
        <p className="text-xs">Querying EVM Testnet RPC...</p>
      </div>
    );
  }

  if (!summary) {
    return (
      <div className="p-12 text-center text-slate-500">
        <p className="text-sm">Enter a Sepolia testnet address above to view wallet metrics and transaction intelligence.</p>
      </div>
    );
  }

  return (
    <div className="p-8 max-w-6xl mx-auto space-y-6">
      {/* Wallet Metric Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="p-5 glass-panel rounded-2xl space-y-2 border-l-4 border-cyan-500">
          <div className="flex items-center justify-between text-xs text-slate-400">
            <span>Testnet Balance</span>
            <Wallet className="w-4 h-4 text-cyan-400" />
          </div>
          <p className="text-2xl font-bold text-white font-mono">{summary.balance_formatted}</p>
          <span className="text-[10px] text-slate-500 font-mono break-all">{summary.balance_wei} Wei</span>
        </div>

        <div className="p-5 glass-panel rounded-2xl space-y-2 border-l-4 border-blue-500">
          <div className="flex items-center justify-between text-xs text-slate-400">
            <span>Tx Count</span>
            <Clock className="w-4 h-4 text-blue-400" />
          </div>
          <p className="text-2xl font-bold text-white font-mono">{summary.tx_count}</p>
          <span className="text-[10px] text-slate-500">Recorded on Sepolia</span>
        </div>

        <div className="p-5 glass-panel rounded-2xl space-y-2 border-l-4 border-purple-500">
          <div className="flex items-center justify-between text-xs text-slate-400">
            <span>Entity Type</span>
            <Tag className="w-4 h-4 text-purple-400" />
          </div>
          <p className="text-xl font-bold text-white uppercase">{summary.entity_type}</p>
          <span className="text-[10px] text-slate-500">{summary.label || 'Unlabeled EOA'}</span>
        </div>

        <div className={`p-5 glass-panel rounded-2xl space-y-2 border-l-4 ${summary.risk_score >= 50 ? 'border-red-500' : 'border-emerald-500'}`}>
          <div className="flex items-center justify-between text-xs text-slate-400">
            <span>Risk Score</span>
            <ShieldAlert className={`w-4 h-4 ${summary.risk_score >= 50 ? 'text-red-400' : 'text-emerald-400'}`} />
          </div>
          <p className={`text-2xl font-bold font-mono ${summary.risk_score >= 50 ? 'text-red-400' : 'text-emerald-400'}`}>
            {summary.risk_score} / 100
          </p>
          <span className="text-[10px] uppercase font-semibold text-slate-400">{summary.risk_level} Level</span>
        </div>
      </div>

      {/* Address Header Banner */}
      <div className="p-6 glass-panel rounded-2xl flex items-center justify-between gap-4">
        <div>
          <span className="text-xs uppercase font-semibold text-cyan-400 tracking-wider">Address Intelligence</span>
          <h2 className="text-lg font-bold font-mono text-white mt-1 break-all">{summary.address}</h2>
        </div>
        <div className="flex gap-3">
          <button
            onClick={() => onTraceAddress(summary.address)}
            className="py-2.5 px-5 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-400 hover:to-blue-500 text-slate-950 font-bold text-xs rounded-xl shadow-lg shadow-cyan-500/20 transition flex items-center gap-2"
          >
            <ArrowUpRight className="w-4 h-4" />
            Launch Multi-Hop Trace
          </button>
          <a
            href={`https://sepolia.etherscan.io/address/${summary.address}`}
            target="_blank"
            rel="noreferrer"
            className="p-2.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl transition flex items-center justify-center"
            title="Etherscan Explorer"
          >
            <ExternalLink className="w-4 h-4" />
          </a>
        </div>
      </div>

      {/* Risk Flags Breakdown */}
      {risk && risk.flags.length > 0 && (
        <div className="p-6 glass-panel rounded-2xl space-y-3">
          <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400 flex items-center gap-2">
            <ShieldAlert className="w-4 h-4 text-amber-400" />
            Triggered Explainable Risk Rules ({risk.flags.length})
          </h3>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {risk.flags.map((flag) => (
              <div key={flag.rule_id} className="p-4 bg-slate-900/80 rounded-xl border border-slate-800 space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-sm text-slate-200">{flag.rule_name}</span>
                  <span className="text-[10px] font-bold px-2 py-0.5 rounded bg-red-500/10 text-red-400 border border-red-500/20">
                    +{flag.score_impact} Score
                  </span>
                </div>
                <p className="text-xs text-slate-400">{flag.description}</p>
                {flag.evidence_detail && (
                  <p className="text-[10px] text-slate-500 font-mono pt-1">Evidence: {flag.evidence_detail}</p>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};
