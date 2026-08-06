import React from 'react';
import { Shield, Radio, Database, Search } from 'lucide-react';

interface HeaderProps {
  currentAddress: string;
  onSearch: (addr: string) => void;
  network: string;
}

export const Header: React.FC<HeaderProps> = ({ currentAddress, onSearch, network }) => {
  const [searchInput, setSearchInput] = React.useState(currentAddress);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (searchInput.trim()) {
      onSearch(searchInput.trim());
    }
  };

  return (
    <header className="h-16 border-b border-slate-800/80 glass-panel px-6 flex items-center justify-between sticky top-0 z-50">
      {/* Brand & Logo */}
      <div className="flex items-center gap-3">
        <div className="w-10 h-10 rounded-xl bg-gradient-to-tr from-cyan-500 to-blue-600 flex items-center justify-center shadow-lg shadow-cyan-500/20">
          <Shield className="w-5 h-5 text-white" />
        </div>
        <div>
          <div className="flex items-center gap-2">
            <span className="font-bold text-lg tracking-wide text-white font-sans">OpenChain</span>
            <span className="text-[10px] uppercase font-semibold px-2 py-0.5 rounded-full bg-cyan-500/10 text-cyan-400 border border-cyan-500/20">
              Intelligence
            </span>
          </div>
          <p className="text-xs text-slate-400">Enterprise Blockchain Investigation</p>
        </div>
      </div>

      {/* Quick Search */}
      <form onSubmit={handleSubmit} className="flex-1 max-w-xl mx-8">
        <div className="relative">
          <Search className="w-4 h-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="Search address (0x...), tx hash, or contract label..."
            className="w-full bg-slate-900/90 border border-slate-700/60 rounded-xl pl-10 pr-4 py-2 text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:border-cyan-500 focus:ring-1 focus:ring-cyan-500 transition font-mono"
          />
        </div>
      </form>

      {/* Network & Live Status */}
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-900/80 border border-slate-800 text-xs">
          <Database className="w-3.5 h-3.5 text-cyan-400" />
          <span className="text-slate-300 font-mono">{network}</span>
        </div>

        <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-xs text-emerald-400">
          <Radio className="w-3.5 h-3.5 animate-pulse" />
          <span className="font-medium">Live EVM Testnet</span>
        </div>
      </div>
    </header>
  );
};
