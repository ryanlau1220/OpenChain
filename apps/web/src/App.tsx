import type React from 'react';
import { useEffect, useState } from 'react';
import { GraphCanvas } from './components/GraphCanvas';
import { Header } from './components/Header';
import { ShareModal } from './components/ShareModal';
import { WalletLookup } from './components/WalletLookup';
import {
	type AddressSummary,
	type GraphData,
	type GraphNode,
	type LabelItem,
	type RiskEvaluation,
	fetchMultiTraceGraph,
	getSharedCanvas,
	lookupAddress,
	shareCanvas,
} from './services/api';

const DEFAULT_SEPOLIA_ADDR = '0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D'; // Uniswap V2 Router on Sepolia

export const App: React.FC = () => {
	const [addresses, setAddresses] = useState<string[]>([DEFAULT_SEPOLIA_ADDR]);
	const [selectedTokens, setSelectedTokens] = useState<string[]>([
		'ETH',
		'USDT',
	]);
	const [summary, setSummary] = useState<AddressSummary | null>(null);
	const [risk, setRisk] = useState<RiskEvaluation | null>(null);
	const [labels, setLabels] = useState<LabelItem[]>([]);
	const [graphData, setGraphData] = useState<GraphData | null>(null);
	const [loading, setLoading] = useState<boolean>(false);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

	// Canvas Share Modal state
	const [shareModalOpen, setShareModalOpen] = useState<boolean>(false);
	const [shareUrl, setShareUrl] = useState<string>('');
	const [shareExpiresAt, setShareExpiresAt] = useState<string>('');

	useEffect(() => {
		// Check URL hash for shared canvas link
		const hash = window.location.hash;
		if (hash.includes('shareId=')) {
			const sId = new URLSearchParams(hash.split('?')[1]).get('shareId');
			if (sId) {
				getSharedCanvas(sId)
					.then((res) => {
						setGraphData(res.graph_data);
					})
					.catch((err) => console.error(err));
				return;
			}
		}

		handleSearch([DEFAULT_SEPOLIA_ADDR], selectedTokens);
	}, []);

	const handleSearch = async (addrs: string[], tokens = selectedTokens) => {
		setAddresses(addrs);
		setSelectedTokens(tokens);
		setLoading(true);
		try {
			if (addrs.length === 1) {
				const data = await lookupAddress(addrs[0]);
				setSummary(data.summary);
				setRisk(data.risk);
				setLabels(data.labels || []);
			} else {
				setSummary(null);
				setRisk(null);
			}

			const gData = await fetchMultiTraceGraph(addrs, 2, 'BOTH', tokens);
			setGraphData(gData);
		} catch (err) {
			console.error(err);
		} finally {
			setLoading(false);
		}
	};

	const handleExpandNode = async (
		nodeAddr: string,
		direction: 'INFLOW' | 'OUTFLOW' | 'BOTH',
	) => {
		try {
			const gData = await fetchMultiTraceGraph(
				[nodeAddr],
				2,
				direction,
				selectedTokens,
			);
			if (gData && graphData) {
				const existingNodeIds = new Set(
					(graphData.nodes || []).map((n) => n.id),
				);
				const newNodes = (gData.nodes || []).filter(
					(n) => !existingNodeIds.has(n.id),
				);

				const existingEdgeIds = new Set(
					(graphData.edges || []).map((e) => e.id),
				);
				const newEdges = (gData.edges || []).filter(
					(e) => !existingEdgeIds.has(e.id),
				);

				const updatedNodes = [...(graphData.nodes || []), ...newNodes];
				const updatedEdges = [...(graphData.edges || []), ...newEdges];

				setGraphData({
					...graphData,
					nodes: updatedNodes,
					edges: updatedEdges,
					total_nodes: updatedNodes.length,
					total_edges: updatedEdges.length,
				});
			}
		} catch (err) {
			console.error(err);
		}
	};

	const handleNodeSelect = async (node: GraphNode | null) => {
		setSelectedNode(node);
		if (node) {
			try {
				const data = await lookupAddress(node.id);
				setSummary(data.summary);
				setRisk(data.risk);
				setLabels(data.labels || []);
			} catch (err) {
				console.error(err);
			}
		}
	};

	const handleShareCanvas = async () => {
		if (!graphData) return;
		try {
			const res = await shareCanvas(graphData);
			const url = `${window.location.origin}${window.location.pathname}#/share?shareId=${res.share_id}`;
			setShareUrl(url);
			setShareExpiresAt(res.expires_at);
			setShareModalOpen(true);
		} catch (err) {
			console.error(err);
		}
	};

	return (
		<div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col font-sans">
			{/* Top Bar with Single / Multi-Address Search & Token Filters */}
			<Header
				currentAddress={addresses[0] || DEFAULT_SEPOLIA_ADDR}
				onSearch={handleSearch}
				network="Sepolia Testnet"
			/>

			{/* Main Single-Page Beosin-Style Investigation Workbench */}
			<div className="flex-1 flex overflow-hidden">
				{/* Graph Flow Visualizer (Center Canvas) */}
				<div className="flex-1 relative">
					<GraphCanvas
						graphData={graphData}
						onNodeSelect={handleNodeSelect}
						onExpandNode={handleExpandNode}
						onShareCanvas={handleShareCanvas}
					/>
				</div>

				{/* Side Inspector Panel (Right Drawer) */}
				<div className="w-80 border-l border-slate-800 bg-slate-900/90 overflow-y-auto p-4 flex flex-col gap-4">
					<h3 className="text-xs uppercase font-bold tracking-wider text-slate-400">
						{selectedNode ? 'Selected Node Detail' : 'Address Inspector'}
					</h3>
					<WalletLookup
						summary={summary}
						risk={risk}
						labels={labels}
						loading={loading}
						onTraceAddress={(addr) => handleSearch([addr], selectedTokens)}
					/>
				</div>
			</div>

			{/* Canvas Sharing Modal */}
			<ShareModal
				isOpen={shareModalOpen}
				onClose={() => setShareModalOpen(false)}
				shareUrl={shareUrl}
				expiresAt={shareExpiresAt}
			/>
		</div>
	);
};
