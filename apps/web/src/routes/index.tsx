import { createFileRoute } from '@tanstack/react-router';
import { useCallback, useEffect, useState } from 'react';
import { GraphCanvas } from '../components/GraphCanvas';
import { Header } from '../components/Header';
import { ShareModal } from '../components/ShareModal';
import { WalletLookup } from '../components/WalletLookup';
import {
	type AddressLabel,
	type AddressSummary,
	type GraphNode,
	type RiskEvaluation,
	TraceGraphResponse,
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
	const [history, setHistory] = useState<TraceGraphResponse[]>([]);
	const [historyIndex, setHistoryIndex] = useState<number>(-1);
	const [loading, setLoading] = useState<boolean>(false);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

	const [shareModalOpen, setShareModalOpen] = useState<boolean>(false);
	const [shareUrl, setShareUrl] = useState<string>('');
	const [shareExpiresAt, setShareExpiresAt] = useState<string>('');

	const graphData =
		historyIndex >= 0 && historyIndex < history.length ? history[historyIndex] : null;

	const pushGraphHistory = useCallback((newGraph: TraceGraphResponse) => {
		setHistory((prev) => {
			const nextHistory = [...prev, newGraph];
			setHistoryIndex(nextHistory.length - 1);
			return nextHistory;
		});
	}, []);

	const loadGraph = useCallback(
		async (addrs: string[], tokens: string[]) => {
			setLoading(true);
			try {
				const gData = await fetchMultiTraceGraph(addrs, 2, undefined, tokens);
				pushGraphHistory(gData);
				if (addrs.length > 0) {
					const mainAddr = addrs[0];
					const lookupRes = await lookupAddress(mainAddr);
					setSummary(lookupRes.summary ?? null);
					setRisk(lookupRes.risk ?? null);
					setLabels(lookupRes.labels || []);
				}
			} catch (err) {
				console.error(err);
			} finally {
				setLoading(false);
			}
		},
		[pushGraphHistory],
	);

	const handleUndo = useCallback(() => {
		setHistoryIndex((prevIdx) => (prevIdx > 0 ? prevIdx - 1 : prevIdx));
	}, []);

	const handleRedo = useCallback(() => {
		setHistoryIndex((prevIdx) => (prevIdx < history.length - 1 ? prevIdx + 1 : prevIdx));
	}, [history.length]);

	useEffect(() => {
		const hash = window.location.hash;
		if (hash.includes('shareId=')) {
			const match = hash.match(/shareId=([^&]+)/);
			if (match?.[1]) {
				getSharedCanvas(match[1]).then((res) => {
					if (res.graphData) {
						pushGraphHistory(res.graphData);
						if (res.graphData.seedAddress) {
							setAddresses([res.graphData.seedAddress]);
						}
					}
				});
				return;
			}
		}
		loadGraph(addresses, selectedTokens);
	}, []);

	const handleSearch = (address: string) => {
		setAddresses([address]);
		loadGraph([address], selectedTokens);
	};

	const handleExpandNode = async (nodeAddr: string) => {
		if (!graphData) return;
		try {
			const expandedRes = await fetchMultiTraceGraph([nodeAddr], 1, undefined, selectedTokens);
			const existingNodeIds = new Set((graphData.nodes || []).map((n) => n.id.toLowerCase()));
			const existingEdgeIds = new Set((graphData.edges || []).map((e) => e.id));

			const newNodes = (expandedRes.nodes || []).filter(
				(n) => !existingNodeIds.has(n.id.toLowerCase()),
			);
			const newEdges = (expandedRes.edges || []).filter((e) => !existingEdgeIds.has(e.id));

			if (newNodes.length > 0 || newEdges.length > 0) {
				const updatedNodes = [...(graphData.nodes || []), ...newNodes];
				const updatedEdges = [...(graphData.edges || []), ...newEdges];
				const updatedGraph = new TraceGraphResponse({
					...graphData,
					nodes: updatedNodes,
					edges: updatedEdges,
					totalNodes: updatedNodes.length,
					totalEdges: updatedEdges.length,
				});
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

		const updatedGraph = new TraceGraphResponse({
			...graphData,
			nodes: keptNodes,
			edges: keptEdges,
			totalNodes: keptNodes.length,
			totalEdges: keptEdges.length,
		});

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
			const url = `${window.location.origin}${window.location.pathname}#/share?shareId=${res.shareId}`;
			setShareUrl(url);
			setShareExpiresAt(res.expiresAt);
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
						onTraceAddress={handleSearch}
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
