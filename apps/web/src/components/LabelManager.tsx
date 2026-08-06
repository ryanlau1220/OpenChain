import React, { useState } from 'react';
import { LabelItem, addLabel } from '../services/api';
import { Tag, Plus, CheckCircle2, ShieldCheck, Link as LinkIcon } from 'lucide-react';

interface LabelManagerProps {
  labels: LabelItem[];
  currentAddress: string;
  onRefresh: () => void;
}

export const LabelManager: React.FC<LabelManagerProps> = ({ labels, currentAddress, onRefresh }) => {
  const [category, setCategory] = useState('DeFi');
  const [label, setLabelText] = useState('');
  const [evidenceUrl, setEvidenceUrl] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!label.trim()) return;

    setSubmitting(true);
    try {
      await addLabel({
        address: currentAddress || '0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D',
        network: 'ETHEREUM_SEPOLIA',
        category,
        label,
        confidence: 1.0,
        evidence_url: evidenceUrl,
        source: 'Analyst Submission',
      });
      setLabelText('');
      setEvidenceUrl('');
      onRefresh();
    } catch (err) {
      console.error(err);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="p-8 max-w-5xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-bold text-white font-sans flex items-center gap-2">
            <Tag className="w-5 h-5 text-cyan-400" />
            Evidence-Backed Address Labels
          </h2>
          <p className="text-xs text-slate-400">Attach provenance-backed tags and metadata to addresses.</p>
        </div>
      </div>

      {/* Label Submission Form */}
      <form onSubmit={handleSubmit} className="p-6 glass-panel rounded-2xl space-y-4">
        <h3 className="text-xs font-bold uppercase tracking-wider text-slate-300">Submit New Label Tag</h3>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label className="block text-xs text-slate-400 mb-1">Target Address</label>
            <input
              type="text"
              value={currentAddress}
              readOnly
              className="w-full bg-slate-900 border border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-cyan-300"
            />
          </div>

          <div>
            <label className="block text-xs text-slate-400 mb-1">Category</label>
            <select
              value={category}
              onChange={(e) => setCategory(e.target.value)}
              className="w-full bg-slate-900 border border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-200"
            >
              <option value="Exchange">Exchange</option>
              <option value="Mixer">Mixer</option>
              <option value="DeFi">DeFi Protocol</option>
              <option value="Sanction">Sanction List</option>
              <option value="Hack">Exploit / Hack</option>
              <option value="Whale">Whale Account</option>
            </select>
          </div>

          <div>
            <label className="block text-xs text-slate-400 mb-1">Label Name</label>
            <input
              type="text"
              placeholder="e.g. Uniswap V3 Pool"
              value={label}
              onChange={(e) => setLabelText(e.target.value)}
              className="w-full bg-slate-900 border border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-200 focus:outline-none focus:border-cyan-500"
            />
          </div>
        </div>

        <div>
          <label className="block text-xs text-slate-400 mb-1">Evidence URL / Provenance Link</label>
          <input
            type="text"
            placeholder="https://etherscan.io/..."
            value={evidenceUrl}
            onChange={(e) => setEvidenceUrl(e.target.value)}
            className="w-full bg-slate-900 border border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-200 focus:outline-none focus:border-cyan-500 font-mono"
          />
        </div>

        <button
          type="submit"
          disabled={submitting}
          className="py-2.5 px-6 bg-cyan-500 hover:bg-cyan-400 text-slate-950 font-bold text-xs rounded-xl shadow-lg shadow-cyan-500/20 transition flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          {submitting ? 'Submitting Tag...' : 'Attach Label Tag'}
        </button>
      </form>

      {/* Label Registry List */}
      <div className="p-6 glass-panel rounded-2xl space-y-4">
        <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400">Registered Labels ({labels.length})</h3>
        <div className="space-y-3">
          {labels.map((lbl) => (
            <div key={lbl.id} className="p-4 bg-slate-900/80 rounded-xl border border-slate-800 flex items-center justify-between">
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <span className="font-bold text-sm text-slate-100">{lbl.label}</span>
                  <span className="text-[10px] uppercase font-semibold px-2 py-0.5 rounded bg-cyan-500/10 text-cyan-400 border border-cyan-500/20">
                    {lbl.category}
                  </span>
                </div>
                <p className="text-xs font-mono text-slate-400">{lbl.address}</p>
              </div>
              <div className="text-right space-y-1">
                <div className="flex items-center gap-1 text-xs text-emerald-400">
                  <ShieldCheck className="w-3.5 h-3.5" />
                  <span>Confidence: {(lbl.confidence * 100).toFixed(0)}%</span>
                </div>
                {lbl.evidence_url && (
                  <a
                    href={lbl.evidence_url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-[10px] text-cyan-400 hover:underline flex items-center justify-end gap-1"
                  >
                    <LinkIcon className="w-3 h-3" />
                    Evidence
                  </a>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
