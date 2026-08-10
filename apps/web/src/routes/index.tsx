import { createFileRoute } from '@tanstack/react-router';
import { useCallback, useEffect, useRef, useState } from 'react';
import { CaseWorkspace } from '../components/CaseWorkspace';
import { EvidencePaths } from '../components/EvidencePaths';
import { GraphCanvas } from '../components/GraphCanvas';
import { Header } from '../components/Header';
import { WalletLookup } from '../components/WalletLookup';
import {
	type AddressLabel,
	type AddressSummary,
	type GraphEdge,
	type GraphNode,
	TraceGraphResponse,
	expandNode,
	fetchTraceGraph,
	lookupAddress,
	requestErrorMessage,
} from '../services/api';
import {
	type LocalCase,
	createLocalCase,
	loadLocalCase,
	saveLocalCase,
} from '../services/case-file';

export const Route = createFileRoute('/')({ component: Index });

type BranchPage = { cursor: string; hasMore: boolean };

function Index() {
	const [address, setAddress] = useState('');
	const [summary, setSummary] = useState<AddressSummary | null>(null);
	const [labels, setLabels] = useState<AddressLabel[]>([]);
	const [graphData, setGraphData] = useState<TraceGraphResponse | null>(null);
	const [loading, setLoading] = useState(false);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
	const [selectedEdge, setSelectedEdge] = useState<GraphEdge | null>(null);
	const [caseFile, setCaseFile] = useState<LocalCase>(createLocalCase);
	const [caseLoaded, setCaseLoaded] = useState(false);
	const [branchPages, setBranchPages] = useState<Record<string, BranchPage>>({});
	const [expandingAddress, setExpandingAddress] = useState<string | null>(null);
	const [pendingExpansion, setPendingExpansion] = useState<string | null>(null);
	const [errorMessage, setErrorMessage] = useState('');
	const investigationRef = useRef(0);

	useEffect(() => {
		setCaseFile(loadLocalCase());
		setCaseLoaded(true);
	}, []);

	useEffect(() => {
		if (caseLoaded) saveLocalCase(caseFile);
	}, [caseFile, caseLoaded]);

	const load = useCallback(async (target: string, preserveCurrentGraph = false) => {
		const investigation = preserveCurrentGraph
			? investigationRef.current
			: ++investigationRef.current;
		if (!preserveCurrentGraph) setLoading(true);
		setErrorMessage('');
		if (!preserveCurrentGraph) {
			setGraphData(null);
			setSelectedNode(null);
			setSelectedEdge(null);
			setSummary(null);
			setLabels([]);
			setBranchPages({});
			setExpandingAddress(null);
			setPendingExpansion(null);
		}
		try {
			const graph = await fetchTraceGraph(target, !preserveCurrentGraph);
			if (investigation !== investigationRef.current) return;
			setGraphData(graph);
			setSelectedNode(graph.nodes.find((node) => node.isSeed) ?? null);
			setCaseFile((current) => ({
				...current,
				rootAddress: graph.seedAddress,
				updatedAt: new Date().toISOString(),
			}));
			setBranchPages({
				[graph.seedAddress.toLowerCase()]: { cursor: graph.nextCursor, hasMore: graph.hasMore },
			});
			void lookupAddress(target).then((lookup) => {
				if (investigation !== investigationRef.current) return;
				setSummary(lookup.summary ?? null);
				setLabels(lookup.labels);
			});
		} catch (error) {
			console.error(error);
			if (investigation === investigationRef.current)
				setErrorMessage(
					requestErrorMessage(
						error,
						'Unable to load this address. Check the backend and Etherscan status.',
					),
				);
		} finally {
			if (!preserveCurrentGraph && investigation === investigationRef.current) setLoading(false);
		}
	}, []);

	useEffect(() => {
		if (!graphData?.pending) return;
		const timer = window.setTimeout(() => void load(graphData.seedAddress, true), 2000);
		return () => window.clearTimeout(timer);
	}, [graphData, load]);

	const handleSelect = useCallback(async (node: GraphNode | null) => {
		setSelectedNode(node);
		setSelectedEdge(null);
		if (node)
			setCaseFile((current) => ({
				...current,
				selectedAddressIds: [...new Set([...current.selectedAddressIds, node.id])],
				updatedAt: new Date().toISOString(),
			}));
		if (!node) return;
		try {
			const lookup = await lookupAddress(node.id);
			setSummary(lookup.summary ?? null);
			setLabels(lookup.labels);
		} catch (error) {
			console.error(error);
		}
	}, []);

	const handleExpand = useCallback(
		async (nodeAddress: string, retry = true) => {
			const key = nodeAddress.toLowerCase();
			const page = branchPages[key];
			if (
				!graphData ||
				expandingAddress ||
				(pendingExpansion && pendingExpansion !== key) ||
				(page && !page.hasMore)
			)
				return;
			const investigation = investigationRef.current;
			setExpandingAddress(key);
			try {
				const expanded = await expandNode(nodeAddress, page?.cursor, retry);
				if (investigation !== investigationRef.current) return;
				if (expanded.pending) {
					setPendingExpansion(key);
					return;
				}
				setPendingExpansion(null);
				setGraphData((current) => {
					if (!current) return current;
					const nodeIds = new Set(current.nodes.map((node) => node.id));
					const edgeIds = new Set(current.edges.map((edge) => edge.id));
					const nodes = [
						...current.nodes,
						...expanded.newNodes.filter((node) => !nodeIds.has(node.id)),
					];
					const edges = [
						...current.edges,
						...expanded.newEdges.filter((edge) => !edgeIds.has(edge.id)),
					];
					return new TraceGraphResponse({
						...current,
						nodes,
						edges,
						totalNodes: nodes.length,
						totalEdges: edges.length,
					});
				});
				setBranchPages((current) => ({
					...current,
					[key]: { cursor: expanded.nextCursor, hasMore: expanded.hasMore },
				}));
			} catch (error) {
				console.error(error);
				if (investigation === investigationRef.current)
					setErrorMessage(
						requestErrorMessage(error, 'Unable to expand this address. Please try again.'),
					);
			} finally {
				if (investigation === investigationRef.current) setExpandingAddress(null);
			}
		},
		[branchPages, expandingAddress, graphData, pendingExpansion],
	);

	useEffect(() => {
		if (!pendingExpansion) return;
		const timer = window.setTimeout(() => void handleExpand(pendingExpansion, false), 2000);
		return () => window.clearTimeout(timer);
	}, [handleExpand, pendingExpansion]);

	const activeAddress = selectedNode?.id || graphData?.seedAddress || '';
	const activeBranch = branchPages[activeAddress.toLowerCase()];
	const canExpand = Boolean(activeAddress) && (!activeBranch || activeBranch.hasMore);

	return (
		<div className="min-h-screen flex flex-col" style={{ background: 'var(--snow)' }}>
			<Header
				currentAddress={address}
				onSearch={(value) => {
					setAddress(value);
					void load(value);
				}}
				network="Ethereum Mainnet"
			/>
			<div className="flex-1 flex overflow-hidden">
				<div className="flex-1 relative">
					<GraphCanvas
						key={graphData?.seedAddress ?? 'empty'}
						graphData={graphData}
						selectedNode={selectedNode}
						onNodeSelect={handleSelect}
						onEdgeSelect={(edge) => {
							setSelectedEdge(edge);
							if (edge)
								setCaseFile((current) => ({
									...current,
									selectedTransferIds: [...new Set([...current.selectedTransferIds, edge.id])],
									updatedAt: new Date().toISOString(),
								}));
						}}
						onExpandNode={handleExpand}
						canExpand={canExpand}
						expanding={
							expandingAddress === activeAddress.toLowerCase() ||
							pendingExpansion === activeAddress.toLowerCase()
						}
					/>
					{!graphData && !loading && (
						<div
							className="absolute inset-0 grid place-items-center pointer-events-none text-sm"
							style={{ color: 'var(--ink-3)' }}
						>
							Search an Ethereum mainnet address to start an investigation.
						</div>
					)}
					{errorMessage && (
						<div
							className="absolute bottom-5 left-5 max-w-md rounded-lg px-3 py-2 text-xs"
							style={{ background: '#fef2f2', color: '#b91c1c', border: '1px solid #fecaca' }}
						>
							{errorMessage}
						</div>
					)}
					{graphData?.sourceStatus?.warning && (
						<div
							className="absolute bottom-5 left-5 max-w-md rounded-lg px-3 py-2 text-xs"
							style={{ background: '#fff7ed', color: '#9a3412', border: '1px solid #fed7aa' }}
						>
							{graphData.sourceStatus.warning}
						</div>
					)}
				</div>
				<div
					className="w-80 overflow-y-auto p-4"
					style={{ borderLeft: '1px solid var(--border)', background: 'rgba(255,255,255,0.70)' }}
				>
					<h3
						className="text-[10px] uppercase font-bold tracking-widest mb-4"
						style={{ color: 'var(--ink-3)' }}
					>
						Address Inspector
					</h3>
					<WalletLookup
						summary={summary}
						labels={labels}
						loading={loading}
						targetSeedAddress={address}
						onTraceAddress={(value) => {
							setAddress(value);
							void load(value);
						}}
					/>
					{graphData && <EvidencePaths nodes={graphData.nodes} edges={graphData.edges} />}
					<CaseWorkspace
						caseFile={caseFile}
						onChange={setCaseFile}
						activeTarget={
							selectedEdge
								? { kind: 'transfer', id: selectedEdge.id }
								: selectedNode
									? { kind: 'address', id: selectedNode.id }
									: null
						}
					/>
				</div>
			</div>
		</div>
	);
}
