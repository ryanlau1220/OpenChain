import { createFileRoute } from '@tanstack/react-router';
import { useCallback, useEffect, useRef, useState } from 'react';
import { CaseWorkspace } from '../components/CaseWorkspace';
import { EvidencePaths } from '../components/EvidencePaths';
import { GraphCanvas } from '../components/GraphCanvas';
import { Header } from '../components/Header';
import { InvestigationLeads } from '../components/InvestigationLeads';
import { TransferInspector } from '../components/TransferInspector';
import { WalletLookup } from '../components/WalletLookup';
import {
	type AddressLabel,
	type AddressSummary,
	type GraphEdge,
	type GraphNode,
	type InvestigationLead,
	Network,
	type SupportedNetwork,
	TraceGraphResponse,
	expandNode,
	exportEvidencePackage,
	fetchTraceGraph,
	fetchTraceStatus,
	lookupAddress,
	networkDetails,
	networkFromSlug,
	requestErrorMessage,
} from '../services/api';
import {
	type LocalCase,
	createLocalCase,
	loadLocalCase,
	saveLocalCase,
} from '../services/case-file';
import {
	type EvidencePackage,
	parseEvidencePackage,
	replayEvidencePackage,
} from '../services/evidence-package';

export const Route = createFileRoute('/')({ component: Index });

type BranchPage = { cursor: string; hasMore: boolean };
const tracePollInterval = 5_000;

function Index() {
	const [address, setAddress] = useState('');
	const [network, setNetwork] = useState<SupportedNetwork>(Network.ETHEREUM_MAINNET);
	const [summary, setSummary] = useState<AddressSummary | null>(null);
	const [labels, setLabels] = useState<AddressLabel[]>([]);
	const [graphData, setGraphData] = useState<TraceGraphResponse | null>(null);
	const [loading, setLoading] = useState(false);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
	const [selectedEdge, setSelectedEdge] = useState<GraphEdge | null>(null);
	const [selectedRelationship, setSelectedRelationship] = useState<readonly GraphEdge[]>([]);
	const [selectedLead, setSelectedLead] = useState<InvestigationLead | null>(null);
	const [highlightedTransferIds, setHighlightedTransferIds] = useState<readonly string[]>([]);
	const [caseFile, setCaseFile] = useState<LocalCase>(createLocalCase);
	const [frozenEvidence, setFrozenEvidence] = useState<EvidencePackage | null>(null);
	const [caseLoaded, setCaseLoaded] = useState(false);
	const [branchPages, setBranchPages] = useState<Record<string, BranchPage>>({});
	const [expandingAddress, setExpandingAddress] = useState<string | null>(null);
	const [pendingExpansion, setPendingExpansion] = useState<string | null>(null);
	const [errorMessage, setErrorMessage] = useState('');
	const investigationRef = useRef(0);

	useEffect(() => {
		const storedCase = loadLocalCase();
		setCaseFile(storedCase);
		setNetwork(networkFromSlug(storedCase.network));
		setCaseLoaded(true);
	}, []);

	useEffect(() => {
		if (caseLoaded) saveLocalCase(caseFile);
	}, [caseFile, caseLoaded]);

	const load = useCallback(
		async (target: string, preserveCurrentGraph = false, targetNetwork = network) => {
			const investigation = preserveCurrentGraph
				? investigationRef.current
				: ++investigationRef.current;
			if (!preserveCurrentGraph) setLoading(true);
			setErrorMessage('');
			if (!preserveCurrentGraph) {
				setGraphData(null);
				setSelectedNode(null);
				setSelectedEdge(null);
				setSelectedRelationship([]);
				setSelectedLead(null);
				setHighlightedTransferIds([]);
				setSummary(null);
				setLabels([]);
				setBranchPages({});
				setExpandingAddress(null);
				setPendingExpansion(null);
				setFrozenEvidence(null);
			}
			try {
				const graph = preserveCurrentGraph
					? await fetchTraceStatus(target, targetNetwork)
					: await fetchTraceGraph(target, targetNetwork, true);
				if (investigation !== investigationRef.current) return;
				setErrorMessage('');
				setGraphData(graph);
				setSelectedNode(graph.nodes.find((node) => node.isSeed) ?? null);
				setCaseFile((current) => ({
					...current,
					network: networkDetails(targetNetwork).slug,
					rootAddress: graph.seedAddress,
					updatedAt: new Date().toISOString(),
				}));
				setBranchPages({
					[graph.seedAddress.toLowerCase()]: { cursor: graph.nextCursor, hasMore: graph.hasMore },
				});
				if (!preserveCurrentGraph)
					void lookupAddress(target, targetNetwork).then((lookup) => {
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
							'Unable to load this address. Check the backend and data-provider status.',
						),
					);
			} finally {
				if (!preserveCurrentGraph && investigation === investigationRef.current) setLoading(false);
			}
		},
		[network],
	);

	const changeNetwork = useCallback(
		(nextNetwork: SupportedNetwork) => {
			if (nextNetwork === network) return;
			investigationRef.current++;
			setNetwork(nextNetwork);
			setGraphData(null);
			setSelectedNode(null);
			setSelectedEdge(null);
			setSelectedRelationship([]);
			setSelectedLead(null);
			setHighlightedTransferIds([]);
			setSummary(null);
			setLabels([]);
			setBranchPages({});
			setExpandingAddress(null);
			setPendingExpansion(null);
			setFrozenEvidence(null);
			setErrorMessage('');
			setCaseFile((current) => ({
				...current,
				network: networkDetails(nextNetwork).slug,
				rootAddress: '',
				selectedAddressIds: [],
				selectedTransferIds: [],
				annotations: [],
				updatedAt: new Date().toISOString(),
			}));
		},
		[network],
	);

	useEffect(() => {
		if (!graphData?.pending) return;
		const timer = window.setTimeout(
			() => void load(graphData.seedAddress, true),
			tracePollInterval,
		);
		return () => window.clearTimeout(timer);
	}, [graphData, load]);

	const handleSelect = useCallback(
		async (node: GraphNode | null) => {
			setSelectedNode(node);
			setSelectedEdge(null);
			setSelectedRelationship([]);
			setSelectedLead(null);
			setHighlightedTransferIds([]);
			if (node)
				setCaseFile((current) => ({
					...current,
					selectedAddressIds: [...new Set([...current.selectedAddressIds, node.id])],
					updatedAt: new Date().toISOString(),
				}));
			if (!node) return;
			try {
				const lookup = await lookupAddress(node.id, network);
				setSummary(lookup.summary ?? null);
				setLabels(lookup.labels);
			} catch (error) {
				console.error(error);
			}
		},
		[network],
	);

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
				const expanded = retry
					? await expandNode(nodeAddress, network, page?.cursor, true)
					: await fetchTraceStatus(nodeAddress, network, page?.cursor).then((status) => ({
							newNodes: status.nodes,
							newEdges: status.edges,
							leads: status.leads,
							nextCursor: status.nextCursor,
							hasMore: status.hasMore,
							pending: status.pending,
						}));
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
					const leadIds = new Set(current.leads.map((lead) => lead.id));
					const leads = [
						...current.leads,
						...expanded.leads.filter((lead) => !leadIds.has(lead.id)),
					];
					return new TraceGraphResponse({
						...current,
						nodes,
						edges,
						leads,
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
		[branchPages, expandingAddress, graphData, network, pendingExpansion],
	);

	useEffect(() => {
		if (!pendingExpansion) return;
		const timer = window.setTimeout(
			() => void handleExpand(pendingExpansion, false),
			tracePollInterval,
		);
		return () => window.clearTimeout(timer);
	}, [handleExpand, pendingExpansion]);

	const activeAddress = selectedNode?.id || graphData?.seedAddress || '';
	const activeBranch = branchPages[activeAddress.toLowerCase()];
	const canExpand = Boolean(activeAddress) && (!activeBranch || activeBranch.hasMore);
	const selectTransfer = useCallback((edge: GraphEdge) => {
		setSelectedEdge(edge);
		setSelectedLead(null);
		setHighlightedTransferIds([edge.id]);
		setCaseFile((current) => ({
			...current,
			selectedTransferIds: [...new Set([...current.selectedTransferIds, edge.id])],
			updatedAt: new Date().toISOString(),
		}));
	}, []);
	const selectLead = useCallback(
		(lead: InvestigationLead) => {
			if (!graphData) return;
			const evidence = graphData.edges.filter((edge) => lead.transferIds.includes(edge.id));
			const subject = graphData.nodes.find((node) => node.id === lead.subjectAddress) ?? null;
			setSelectedLead(lead);
			setSelectedRelationship(evidence);
			setSelectedEdge(evidence[0] ?? null);
			setSelectedNode(subject);
			setHighlightedTransferIds(lead.transferIds);
			setCaseFile((current) => ({
				...current,
				selectedAddressIds: subject
					? [...new Set([...current.selectedAddressIds, subject.id])]
					: current.selectedAddressIds,
				selectedTransferIds: [...new Set([...current.selectedTransferIds, ...lead.transferIds])],
				updatedAt: new Date().toISOString(),
			}));
			if (subject)
				void lookupAddress(subject.id, network).then((lookup) => {
					setSummary(lookup.summary ?? null);
					setLabels(lookup.labels);
				});
		},
		[graphData, network],
	);
	const exportFrozenEvidence = useCallback(async () => {
		if (!graphData) throw new Error('Trace an address before creating an evidence package.');
		const graphIDs = new Set(graphData.edges.map((edge) => edge.id));
		const selected = caseFile.selectedTransferIds.filter((id) => graphIDs.has(id));
		const transferIDs = selected.length > 0 ? selected : graphData.edges.map((edge) => edge.id);
		if (transferIDs.length === 0) throw new Error('This investigation has no transfers to export.');
		const packageJSON = await exportEvidencePackage(network, transferIDs, JSON.stringify(caseFile));
		const packageFile = await parseEvidencePackage(packageJSON);
		setFrozenEvidence(packageFile);
		return packageJSON;
	}, [caseFile, graphData, network]);
	const importFrozenEvidence = useCallback((packageFile: EvidencePackage) => {
		const replay = replayEvidencePackage(packageFile);
		investigationRef.current++;
		setAddress(replay.caseFile.rootAddress);
		setNetwork(replay.network);
		setCaseFile(replay.caseFile);
		setGraphData(replay.graph);
		setSelectedNode(replay.graph.nodes.find((node) => node.isSeed) ?? null);
		setSelectedEdge(null);
		setSelectedRelationship([]);
		setSelectedLead(null);
		setHighlightedTransferIds([]);
		setSummary(null);
		setLabels([]);
		setBranchPages({});
		setExpandingAddress(null);
		setPendingExpansion(null);
		setErrorMessage('');
		setFrozenEvidence(packageFile);
	}, []);

	return (
		<div className="h-dvh overflow-hidden flex flex-col" style={{ background: 'var(--snow)' }}>
			{caseLoaded && (
				<Header
					currentAddress={address}
					onSearch={(value, detectedNetwork) => {
						const targetNetwork = detectedNetwork ?? network;
						changeNetwork(targetNetwork);
						setAddress(value);
						void load(value, false, targetNetwork);
					}}
					network={network}
					onNetworkChange={changeNetwork}
				/>
			)}
			<div className="min-h-0 flex-1 flex flex-col md:flex-row overflow-hidden">
				<div className="min-h-0 min-w-0 flex-1 relative">
					<GraphCanvas
						key={`${network}:${graphData?.seedAddress ?? 'empty'}`}
						graphData={graphData}
						loading={loading}
						emptyMessage={`Search a ${networkDetails(network).name} address to start an investigation.`}
						selectedNode={selectedNode}
						onNodeSelect={handleSelect}
						onEdgeSelect={(edge) => {
							if (edge) selectTransfer(edge);
							else setSelectedEdge(null);
						}}
						onRelationshipSelect={(edges) => {
							setSelectedRelationship(edges);
							if (edges.length > 0) setHighlightedTransferIds(edges.map((edge) => edge.id));
							else setHighlightedTransferIds([]);
						}}
						highlightedTransferIds={highlightedTransferIds}
						onExpandNode={handleExpand}
						canExpand={canExpand}
						expanding={
							expandingAddress === activeAddress.toLowerCase() ||
							pendingExpansion === activeAddress.toLowerCase()
						}
					/>
					{(errorMessage || graphData?.sourceStatus?.warning) && (
						<div className="absolute bottom-5 right-5 z-10 max-w-md space-y-2 text-xs pointer-events-none">
							{errorMessage && (
								<div
									className="rounded-lg px-3 py-2"
									style={{ background: '#fef2f2', color: '#b91c1c', border: '1px solid #fecaca' }}
								>
									{errorMessage}
								</div>
							)}
							{graphData?.sourceStatus?.warning && (
								<div
									className="rounded-lg px-3 py-2"
									style={{ background: '#fff7ed', color: '#9a3412', border: '1px solid #fed7aa' }}
								>
									{graphData.sourceStatus.warning}
								</div>
							)}
						</div>
					)}
				</div>
				<div
					className="w-full max-h-[42dvh] md:max-h-none md:w-80 shrink-0 overflow-y-auto p-4"
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
						network={network}
						targetSeedAddress={address}
						onTraceAddress={(value) => {
							setAddress(value);
							void load(value);
						}}
					/>
					<TransferInspector
						edge={selectedEdge}
						edges={selectedRelationship}
						network={network}
						onSelect={selectTransfer}
					/>
					{graphData && (
						<InvestigationLeads
							leads={graphData.leads}
							edges={graphData.edges}
							selectedLeadId={selectedLead?.id}
							onSelect={selectLead}
						/>
					)}
					{graphData && (
						<EvidencePaths nodes={graphData.nodes} edges={graphData.edges} network={network} />
					)}
					{caseLoaded && (
						<CaseWorkspace
							caseFile={caseFile}
							onChange={setCaseFile}
							onExportEvidence={exportFrozenEvidence}
							onImportEvidence={importFrozenEvidence}
							frozenEvidence={frozenEvidence}
							activeTarget={
								selectedEdge
									? { kind: 'transfer', id: selectedEdge.id }
									: selectedNode
										? { kind: 'address', id: selectedNode.id }
										: null
							}
						/>
					)}
				</div>
			</div>
		</div>
	);
}
