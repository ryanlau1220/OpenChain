import React, { useEffect, useState } from 'react';
import { InvestigationCase, fetchCases, createCase, getExportUrl } from '../services/api';
import { FolderKanban, Plus, Download, FileJson, FileText, CheckCircle2, Clock } from 'lucide-react';

export const CaseWorkbench: React.FC = () => {
  const [cases, setCases] = useState<InvestigationCase[]>([]);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    loadCases();
  }, []);

  const loadCases = async () => {
    try {
      const data = await fetchCases();
      setCases(data);
    } catch (err) {
      console.error(err);
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;

    setSubmitting(true);
    try {
      await createCase({
        title,
        description,
        tags: ['Sepolia', 'Investigation'],
      });
      setTitle('');
      setDescription('');
      loadCases();
    } catch (err) {
      console.error(err);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="p-8 max-w-5xl mx-auto space-y-6">
      <div>
        <h2 className="text-xl font-bold text-white font-sans flex items-center gap-2">
          <FolderKanban className="w-5 h-5 text-cyan-400" />
          Investigation Case Workbench
        </h2>
        <p className="text-xs text-slate-400">Manage investigation dossiers and export evidence reports (PDF/CSV/JSON).</p>
      </div>

      {/* Create Case Form */}
      <form onSubmit={handleCreate} className="p-6 glass-panel rounded-2xl space-y-4">
        <h3 className="text-xs font-bold uppercase tracking-wider text-slate-300">Create New Investigation Case</h3>
        <div className="space-y-3">
          <div>
            <label className="block text-xs text-slate-400 mb-1">Case Title</label>
            <input
              type="text"
              placeholder="e.g. Sepolia Testnet High-Value Transfer Audit"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="w-full bg-slate-900 border border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-200 focus:outline-none focus:border-cyan-500"
            />
          </div>

          <div>
            <label className="block text-xs text-slate-400 mb-1">Description & Scope</label>
            <textarea
              placeholder="Detail the target wallets, suspicious tx hashes, and evidence requirements..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="w-full bg-slate-900 border border-slate-700 rounded-xl px-3 py-2 text-xs text-slate-200 focus:outline-none focus:border-cyan-500 h-20"
            />
          </div>
        </div>

        <button
          type="submit"
          disabled={submitting}
          className="py-2.5 px-6 bg-cyan-500 hover:bg-cyan-400 text-slate-950 font-bold text-xs rounded-xl shadow-lg shadow-cyan-500/20 transition flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          {submitting ? 'Creating Case...' : 'Create Case'}
        </button>
      </form>

      {/* Cases List */}
      <div className="space-y-4">
        <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400">Active Investigation Dossiers ({cases.length})</h3>
        {cases.map((c) => (
          <div key={c.id} className="p-6 glass-panel rounded-2xl flex items-center justify-between gap-4 border-l-4 border-cyan-500">
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <span className="font-mono font-bold text-xs text-cyan-400">{c.id}</span>
                <h4 className="font-bold text-base text-slate-100">{c.title}</h4>
                <span className="text-[10px] uppercase font-semibold px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                  {c.status}
                </span>
              </div>
              <p className="text-xs text-slate-400">{c.description}</p>
              <div className="flex items-center gap-4 text-[10px] text-slate-500 font-mono">
                <span>Investigator: {c.created_by}</span>
                <span>Created: {new Date(c.created_at).toLocaleDateString()}</span>
              </div>
            </div>

            <div className="flex gap-2">
              <a
                href={getExportUrl(c.id, 'JSON')}
                download
                className="py-2 px-3 bg-slate-900 hover:bg-slate-800 border border-slate-700 text-slate-300 text-xs font-semibold rounded-xl transition flex items-center gap-1.5"
              >
                <FileJson className="w-3.5 h-3.5 text-cyan-400" />
                Export JSON
              </a>
              <a
                href={getExportUrl(c.id, 'CSV')}
                download
                className="py-2 px-3 bg-slate-900 hover:bg-slate-800 border border-slate-700 text-slate-300 text-xs font-semibold rounded-xl transition flex items-center gap-1.5"
              >
                <FileText className="w-3.5 h-3.5 text-emerald-400" />
                Export CSV
              </a>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
