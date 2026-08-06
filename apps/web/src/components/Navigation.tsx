import React from 'react';
import { GitFork, Wallet, Tag, ShieldAlert, FolderKanban, Bell } from 'lucide-react';

export type NavTab = 'graph' | 'wallet' | 'labels' | 'risk' | 'cases' | 'watchlist';

interface NavigationProps {
  activeTab: NavTab;
  onSelectTab: (tab: NavTab) => void;
}

export const Navigation: React.FC<NavigationProps> = ({ activeTab, onSelectTab }) => {
  const tabs: { id: NavTab; label: string; icon: React.ReactNode }[] = [
    { id: 'graph', label: 'Graph Canvas', icon: <GitFork className="w-4 h-4" /> },
    { id: 'wallet', label: 'Wallet Lookup', icon: <Wallet className="w-4 h-4" /> },
    { id: 'labels', label: 'Label Registry', icon: <Tag className="w-4 h-4" /> },
    { id: 'risk', label: 'Risk Rules', icon: <ShieldAlert className="w-4 h-4" /> },
    { id: 'cases', label: 'Case Workbench', icon: <FolderKanban className="w-4 h-4" /> },
    { id: 'watchlist', label: 'Watchlist Feed', icon: <Bell className="w-4 h-4" /> },
  ];

  return (
    <nav className="h-12 border-b border-slate-800/60 bg-slate-950/60 backdrop-blur-md px-6 flex items-center gap-1">
      {tabs.map((tab) => {
        const isActive = activeTab === tab.id;
        return (
          <button
            key={tab.id}
            onClick={() => onSelectTab(tab.id)}
            className={`flex items-center gap-2 px-4 py-1.5 rounded-lg text-xs font-medium transition-all ${
              isActive
                ? 'bg-cyan-500/15 text-cyan-400 border border-cyan-500/30 shadow-sm'
                : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900/60'
            }`}
          >
            {tab.icon}
            <span>{tab.label}</span>
          </button>
        );
      })}
    </nav>
  );
};
