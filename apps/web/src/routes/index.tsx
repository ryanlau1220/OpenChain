import { createFileRoute } from '@tanstack/react-router';
import { useCallback, useEffect, useState } from 'react';
import { GraphCanvas } from '../components/GraphCanvas';
import { Header } from '../components/Header';
import { WalletLookup } from '../components/WalletLookup';
import { type AddressLabel, type AddressSummary, type GraphNode, TraceGraphResponse, expandNode, fetchTraceGraph, lookupAddress } from '../services/api';

export const Route = createFileRoute('/')({ component: Index });

const DEFAULT_SEPOLIA_ADDR = '0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D';

function Index() {
	const [address, setAddress] = useState(DEFAULT_SEPOLIA_ADDR);
	const [summary, setSummary] = useState<AddressSummary | null>(null);
	const [labels, setLabels] = useState<AddressLabel[]>([]);
	const [graphData, setGraphData] = useState<TraceGraphResponse | null>(null);
	const [loading, setLoading] = useState(false);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

	const load = useCallback(async (target: string) => {
		setLoading(true);
		try {
			const [graph, lookup] = await Promise.all([fetchTraceGraph(target), lookupAddress(target)]);
			setGraphData(graph);
			setSummary(lookup.summary ?? null);
			setLabels(lookup.labels);
			setSelectedNode(graph.nodes.find((node) => node.isSeed) ?? null);
		} catch (error) { console.error(error); } finally { setLoading(false); }
	}, []);

	useEffect(() => { void load(address); }, []);

	const handleSelect = useCallback(async (node: GraphNode | null) => {
		setSelectedNode(node);
		if (!node) return;
		try {
			const lookup = await lookupAddress(node.id);
			setSummary(lookup.summary ?? null);
			setLabels(lookup.labels);
		} catch (error) { console.error(error); }
	}, []);

	const handleExpand = useCallback(async (nodeAddress: string) => {
		if (!graphData) return;
		try {
			const expanded = await expandNode(nodeAddress);
			const nodeIds = new Set(graphData.nodes.map((node) => node.id));
			const edgeIds = new Set(graphData.edges.map((edge) => edge.id));
			const nodes = [...graphData.nodes, ...expanded.newNodes.filter((node) => !nodeIds.has(node.id))];
			const edges = [...graphData.edges, ...expanded.newEdges.filter((edge) => !edgeIds.has(edge.id))];
			setGraphData(new TraceGraphResponse({ ...graphData, nodes, edges, totalNodes: nodes.length, totalEdges: edges.length }));
		} catch (error) { console.error(error); }
	}, [graphData]);

	return <div className="min-h-screen flex flex-col" style={{ background: 'var(--snow)' }}>
		<Header currentAddress={address} onSearch={(value) => { setAddress(value); void load(value); }} network="Sepolia Testnet" />
		<div className="flex-1 flex overflow-hidden">
			<div className="flex-1 relative"><GraphCanvas graphData={graphData} selectedNode={selectedNode} onNodeSelect={handleSelect} onExpandNode={handleExpand} /></div>
			<div className="w-80 overflow-y-auto p-4" style={{ borderLeft: '1px solid var(--border)', background: 'rgba(255,255,255,0.70)' }}>
				<h3 className="text-[10px] uppercase font-bold tracking-widest mb-4" style={{ color: 'var(--ink-3)' }}>Address Inspector</h3>
				<WalletLookup summary={summary} labels={labels} loading={loading} targetSeedAddress={address} onTraceAddress={(value) => { setAddress(value); void load(value); }} />
			</div>
		</div>
	</div>;
}
