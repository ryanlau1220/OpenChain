import { createFileRoute } from '@tanstack/react-router';
import { useEffect, useState } from 'react';
import { GraphCanvas } from '../components/GraphCanvas';
import { Header } from '../components/Header';
import { ShareModal } from '../components/ShareModal';
import { WalletLookup } from '../components/WalletLookup';
import {
	type AddressLabel,
	type AddressSummary,
	type GraphData,
	type GraphNode,
	type RiskEvaluation,
	fetchMultiTraceGraph,
	getSharedCanvas,
	lookupAddress,
	shareCanvas,
} from '../services/api';

export const Route = createFileRoute('/')({
	component: Index,
});

const DEFAULT_SEPOLIA_ADDR = '0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D';

function Index() {
	const [addresses, setAddresses] = useState<string[]>([DEFAULT_SEPOLIA_ADDR]);
	const [selectedTokens, setSelectedTokens] = useState<string[]>(['ETH', 'USDT']);
	const [summary, setSummary] = useState<AddressSummary | null>(null);
	const [risk, setRisk] = useState<RiskEvaluation | null>(null);
	const [labels, setLabels] = useState<AddressLabel[]>([]);
	const [graphData, setGraphData] = useState<GraphData | null>(null);
	const [loading, setLoading] = useState<boolean>(false);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

	const [shareModalOpen, setShareModalOpen] = useState<boolean>(false);
	const [shareUrl, setShareUrl] = useState<string>('');
	const [shareExpiresAt, setShareExpiresAt] = useState<string>('');

	useEffect(() => {
		if (typeof window !== 'undefined') {
			const hash = window.location.hash;
			if (hash.includes('shareId=')) {
				const sId = new URLSearchParams(hash.split('?')[1]).get('shareId');
				if (sId) {
					getSharedCanvas(sId)
						.then((res) => setGraphData(res.graph_data))
						.catch((err) => console.error(err));
					return;
				}
			}
		}
		handleSearch([DEFAULT_SEPOLIA_ADDR], selectedTokens);
	}, []);

	const handleSearch = async (addrs: string[], tokens = selectedTokens) => {
		setAddresses(addrs);
		setSelectedTokens(tokens);
		setLoading(true);

		const lookupPromise = (async () => {
			if (addrs.length === 1) {
				try {
					const data = await lookupAddress(addrs[0]);
					setSummary(data.summary ?? null);
					setRisk(data.risk ?? null);
					setLabels(data.labels || []);
				} catch (err) {
					console.error('Lookup address failed:', err);
				}
			} else {
				setSummary(null);
				setRisk(null);
			}
		})();

		const tracePromise = (async () => {
			try {
				const gData = await fetchMultiTraceGraph(addrs, 2, 'BOTH', tokens);
				setGraphData(gData);
			} catch (err) {
				console.error('Fetch graph trace failed:', err);
			}
		})();

		await Promise.allSettled([lookupPromise, tracePromise]);
		setLoading(false);
	};

	const handleExpandNode = async (nodeAddr: string, direction: 'INFLOW' | 'OUTFLOW' | 'BOTH') => {
		try {
			const gData = await fetchMultiTraceGraph([nodeAddr], 2, direction, selectedTokens);
			if (gData && graphData) {
				const existingNodeIds = new Set((graphData.nodes || []).map((n) => n.id));
				const newNodes = (gData.nodes || []).filter((n) => !existingNodeIds.has(n.id));
				const existingEdgeIds = new Set((graphData.edges || []).map((e) => e.id));
				const newEdges = (gData.edges || []).filter((e) => !existingEdgeIds.has(e.id));
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
				setSummary(data.summary ?? null);
				setRisk(data.risk ?? null);
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
		<div className="min-h-screen flex flex-col" style={{ background: 'var(--snow)' }}>
			<Header
				currentAddress={addresses[0] || DEFAULT_SEPOLIA_ADDR}
				onSearch={handleSearch}
				network="Sepolia Testnet"
			/>

			<div className="flex-1 flex overflow-hidden">
				{/* Graph area */}
				<div className="flex-1 relative">
					<GraphCanvas
						graphData={graphData}
						onNodeSelect={handleNodeSelect}
						onExpandNode={handleExpandNode}
						onShareCanvas={handleShareCanvas}
					/>
				</div>

				{/* Inspector sidebar */}
				<div
					className="w-80 overflow-y-auto p-4 flex flex-col gap-4"
					style={{
						borderLeft: '1px solid var(--border)',
						background: 'rgba(255,255,255,0.70)',
						backdropFilter: 'blur(8px)',
					}}
				>
					<h3
						className="text-[10px] uppercase font-bold tracking-widest"
						style={{ color: 'var(--ink-3)' }}
					>
						{selectedNode ? 'Selected Node' : 'Address Inspector'}
					</h3>
					<WalletLookup
						summary={summary}
						risk={risk}
						labels={labels}
						loading={loading}
						onTraceAddress={(addr: string) => handleSearch([addr], selectedTokens)}
					/>
				</div>
			</div>

			<ShareModal
				isOpen={shareModalOpen}
				onClose={() => setShareModalOpen(false)}
				shareUrl={shareUrl}
				expiresAt={shareExpiresAt}
			/>
		</div>
	);
}
