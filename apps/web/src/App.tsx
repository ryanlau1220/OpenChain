import React, { useState, useEffect } from 'react';
import { Header } from './components/Header';
import { Navigation, NavTab } from './components/Navigation';
import { GraphCanvas } from './components/GraphCanvas';
import { WalletLookup } from './components/WalletLookup';
import { LabelManager } from './components/LabelManager';
import { RiskDrawer } from './components/RiskDrawer';
import { CaseWorkbench } from './components/CaseWorkbench';
import { WatchlistPanel } from './components/WatchlistPanel';
import { lookupAddress, fetchTraceGraph, AddressSummary, RiskEvaluation, LabelItem, GraphData, GraphNode } from './services/api';

const DEFAULT_SEPOLIA_ADDR = '0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D'; // Uniswap V2 Router on Sepolia

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<NavTab>('graph');
  const [currentAddress, setCurrentAddress] = useState<string>(DEFAULT_SEPOLIA_ADDR);

  const [summary, setSummary] = useState<AddressSummary | null>(null);
  const [risk, setRisk] = useState<RiskEvaluation | null>(null);
  const [labels, setLabels] = useState<LabelItem[]>([]);
  const [graphData, setGraphData] = useState<GraphData | null>(null);
  const [loading, setLoading] = useState<boolean>(false);

  useEffect(() => {
    handleSearch(currentAddress);
  }, []);

  const handleSearch = async (address: string) => {
    setCurrentAddress(address);
    setLoading(true);
    try {
      // Lookup Address Info
      const data = await lookupAddress(address);
      setSummary(data.summary);
      setRisk(data.risk);
      setLabels(data.labels || []);

      // Fetch Graph Tracing Data
      const gData = await fetchTraceGraph(address, 2);
      setGraphData(gData);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const handleExpandNode = async (nodeAddr: string) => {
    try {
      const gData = await fetchTraceGraph(nodeAddr, 2);
      if (gData && graphData) {
        // Merge nodes and edges
        const existingNodeIds = new Set(graphData.nodes.map(n => n.id));
        const newNodes = gData.nodes.filter(n => !existingNodeIds.has(n.id));

        const existingEdgeIds = new Set(graphData.edges.map(e => e.id));
        const newEdges = gData.edges.filter(e => !existingEdgeIds.has(e.id));

        setGraphData({
          ...graphData,
          nodes: [...graphData.nodes, ...newNodes],
          edges: [...graphData.edges, ...newEdges],
          total_nodes: graphData.nodes.length + newNodes.length,
          total_edges: graphData.edges.length + newEdges.length,
        });
      }
    } catch (err) {
      console.error(err);
    }
  };

  return (
    <div className="min-h-screen bg-background text-slate-100 flex flex-col font-sans">
      <Header
        currentAddress={currentAddress}
        onSearch={handleSearch}
        network="Sepolia Testnet"
      />
      <Navigation activeTab={activeTab} onSelectTab={setActiveTab} />

      <main className="flex-1 overflow-x-hidden">
        {activeTab === 'graph' && (
          <GraphCanvas
            graphData={graphData}
            onNodeSelect={(node) => handleSearch(node.id)}
            onExpandNode={handleExpandNode}
          />
        )}

        {activeTab === 'wallet' && (
          <WalletLookup
            summary={summary}
            risk={risk}
            labels={labels}
            loading={loading}
            onTraceAddress={(addr) => {
              setCurrentAddress(addr);
              setActiveTab('graph');
              handleSearch(addr);
            }}
          />
        )}

        {activeTab === 'labels' && (
          <LabelManager
            labels={labels}
            currentAddress={currentAddress}
            onRefresh={() => handleSearch(currentAddress)}
          />
        )}

        {activeTab === 'risk' && <RiskDrawer />}
        {activeTab === 'cases' && <CaseWorkbench />}
        {activeTab === 'watchlist' && <WatchlistPanel />}
      </main>
    </div>
  );
};
