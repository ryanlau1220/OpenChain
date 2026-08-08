import { createFileRoute } from '@tanstack/react-router';
import { useCallback, useEffect, useState } from 'react';
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
	const [history, setHistory] = useState<GraphData[]>([]);
	const [historyIndex, setHistoryIndex] = useState<number>(-1);
	const [loading, setLoading] = useState<boolean>(false);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

	const [shareModalOpen, setShareModalOpen] = useState<boolean>(false);
	const [shareUrl, setShareUrl] = useState<string>('');
	const [shareExpiresAt, setShareExpiresAt] = useState<string>('');

	const graphData =
		historyIndex >= 0 && historyIndex < history.length ? history[historyIndex] : null;

	const pushGraphHistory = useCallback((newGraph: GraphData) => {
		setHistory((prev) => {
			const nextHistory = [...prev, newGraph];
			setHistoryIndex(nextHistory.length - 1);
			return nextHistory;
		});
	}, []);

	const handleUndo = useCallback(() => {
		setHistoryIndex((prevIdx) => (prevIdx > 0 ? prevIdx - 1 : prevIdx));
	}, []);

	const handleRedo = useCallback(() => {
		setHistoryIndex((prevIdx) => (prevIdx < history.length - 1 ? prevIdx + 1 : prevIdx));
	}, [history.length]);

	useEffect(() => {
		if (typeof window !== 'undefined') {
			const hash = window.location.hash;
			if (hash.includes('shareId=')) {
				const sId = new URLSearchParams(hash.split('?')[1]).get('shareId');
				if (sId) {
					getSharedCanvas(sId)
						.then((res) => pushGraphHistory(res.graph_data))
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
				if (gData) {
					pushGraphHistory(gData);
				}
			} catch (err) {
				console.error('Fetch graph trace failed:', err);
			}
		})();

		await Promise.allSettled([lookupPromise, tracePromise]);
		setLoading(false);
	};

	const handleExpandNode = async (nodeAddr: string) => {
		try {
			const gData = await fetchMultiTraceGraph([nodeAddr], 2, 'BOTH', selectedTokens);
			if (gData && graphData) {
				const existingNodeIds = new Set((graphData.nodes || []).map((n) => n.id));
				const newNodes = (gData.nodes || []).filter((n) => !existingNodeIds.has(n.id));
				const existingEdgeIds = new Set((graphData.edges || []).map((e) => e.id));
				const newEdges = (gData.edges || []).filter((e) => !existingEdgeIds.has(e.id));
				const updatedNodes = [...(graphData.nodes || []), ...newNodes];
				const updatedEdges = [...(graphData.edges || []), ...newEdges];
				const updatedGraph = {
					...graphData,
					nodes: updatedNodes,
					edges: updatedEdges,
					total_nodes: updatedNodes.length,
					total_edges: updatedEdges.length,
				};
				pushGraphHistory(updatedGraph);
			}
		} catch (err) {
			console.error(err);
		}
	};

	const handleCollapseNode = (nodeAddr: string) => {
		if (!graphData) return;
		const cleanAddr = nodeAddr.toLowerCase();

		const keptEdges = (graphData.edges || []).filter(
			(e) => e.source.toLowerCase() !== cleanAddr && e.target.toLowerCase() !== cleanAddr,
		);

		const connectedNodeIds = new Set<string>();
		keptEdges.forEach((e) => {
			connectedNodeIds.add(e.source.toLowerCase());
			connectedNodeIds.add(e.target.toLowerCase());
		});

		const keptNodes = (graphData.nodes || []).filter((n) => {
			const nId = n.id.toLowerCase();
			return n.isSeed || nId === cleanAddr || connectedNodeIds.has(nId);
		});

		const updatedGraph = {
			...graphData,
			nodes: keptNodes,
			edges: keptEdges,
			total_nodes: keptNodes.length,
			total_edges: keptEdges.length,
		};

		pushGraphHistory(updatedGraph);
	};

	const handleNodeSelect = useCallback(async (node: GraphNode | null) => {
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
	}, []);

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
						selectedNode={selectedNode}
						onNodeSelect={handleNodeSelect}
						onExpandNode={handleExpandNode}
						onCollapseNode={handleCollapseNode}
						onShareCanvas={handleShareCanvas}
						onUndo={handleUndo}
						onRedo={handleRedo}
						canUndo={historyIndex > 0}
						canRedo={historyIndex < history.length - 1}
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
						{selectedNode
							? selectedNode.id.toLowerCase() === (addresses[0] || '').toLowerCase()
								? 'Target Address Inspector'
								: 'Selected Node Inspector'
							: 'Address Inspector'}
					</h3>
					<WalletLookup
						summary={summary}
						risk={risk}
						labels={labels}
						loading={loading}
						targetSeedAddress={addresses[0] || (graphData?.nodes || []).find((n) => n.isSeed)?.id}
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
