import { createFileRoute } from '@tanstack/react-router';
import { useCallback, useEffect, useRef, useState } from 'react';
import { CaseWorkspace } from '../components/CaseWorkspace';
import { CrossChainPaths } from '../components/CrossChainPaths';
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
	type GraphOptions,
	type InvestigationLead,
	type LookupFieldStatus,
	Network,
	type NetworkCapabilities,
	type NetworkSlug,
	type SupportedNetwork,
	type TraceDirection,
	TraceGraphResponse,
	defaultGraphOptions,
	expandNode,
	exportEvidencePackage,
	fetchNetworkCapabilities,
	fetchTraceGraph,
	fetchTraceStatus,
	isNetworkSlug,
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
import { nodeDepths } from '../services/graph-scope';

export const Route = createFileRoute('/')({ component: Index });

type BranchPage = { cursor: string; hasMore: boolean };
type CaseUpdate = LocalCase | ((current: LocalCase) => LocalCase);
type InspectorTab = 'address' | 'findings' | 'evidence' | 'case';
const tracePollInterval = 5_000;
function allowedNumber(value: string | null, allowed: readonly number[], fallback: number): number {
	const parsed = Number(value);
	return allowed.includes(parsed) ? parsed : fallback;
}

function scopeFromQuery(params: URLSearchParams, fallback: GraphOptions): GraphOptions {
	return {
		...fallback,
		maxCounterparties: allowedNumber(
			params.get('counterparties'),
			[5, 10, 25],
			fallback.maxCounterparties,
		),
		ranking: allowedNumber(
			params.get('ranking'),
			[1, 2, 3, 4],
			fallback.ranking,
		) as GraphOptions['ranking'],
		direction: allowedNumber(
			params.get('direction'),
			[1, 2, 3],
			fallback.direction,
		) as GraphOptions['direction'],
		maxDepth: allowedNumber(params.get('depth'), [1, 2, 3], fallback.maxDepth),
		from: params.get('from') ?? fallback.from,
		to: params.get('to') ?? fallback.to,
		asset: params.get('asset') ?? fallback.asset,
		minimumAmount: params.get('minimumAmount') ?? fallback.minimumAmount,
		transferKind: params.get('transferKind') ?? fallback.transferKind,
	};
}

function Index() {
	const [address, setAddress] = useState('');
	const [network, setNetwork] = useState<SupportedNetwork>(Network.ETHEREUM_MAINNET);
	const [graphOptions, setGraphOptions] = useState<GraphOptions>(defaultGraphOptions);
	const [summary, setSummary] = useState<AddressSummary | null>(null);
	const [labels, setLabels] = useState<AddressLabel[]>([]);
	const [fieldStatuses, setFieldStatuses] = useState<LookupFieldStatus[]>([]);
	const [networkCapabilities, setNetworkCapabilities] = useState<
		Partial<Record<NetworkSlug, NetworkCapabilities>>
	>({});
	const [graphData, setGraphData] = useState<TraceGraphResponse | null>(null);
	const [loading, setLoading] = useState(false);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
	const [selectedEdge, setSelectedEdge] = useState<GraphEdge | null>(null);
	const [selectedRelationship, setSelectedRelationship] = useState<readonly GraphEdge[]>([]);
	const [selectedLead, setSelectedLead] = useState<InvestigationLead | null>(null);
	const [inspectorTab, setInspectorTab] = useState<InspectorTab>('address');
	const [highlightedTransferIds, setHighlightedTransferIds] = useState<readonly string[]>([]);
	const [caseFile, setCaseFile] = useState<LocalCase>(createLocalCase);
	const caseFileRef = useRef(caseFile);
	const [frozenEvidence, setFrozenEvidence] = useState<EvidencePackage | null>(null);
	const [caseLoaded, setCaseLoaded] = useState(false);
	const [branchPages, setBranchPages] = useState<Record<string, BranchPage>>({});
	const [expandingAddress, setExpandingAddress] = useState<string | null>(null);
	const [pendingExpansion, setPendingExpansion] = useState<string | null>(null);
	const [errorMessage, setErrorMessage] = useState('');
	const investigationRef = useRef(0);
	const selectionRef = useRef(0);
	const graphOptionsRef = useRef(graphOptions);
	const initialSearchRef = useRef<{
		address: string;
		network: SupportedNetwork;
		scope: GraphOptions;
	} | null>(null);
	const updateCaseFile = useCallback((update: CaseUpdate) => {
		const next = typeof update === 'function' ? update(caseFileRef.current) : update;
		caseFileRef.current = next;
		setCaseFile(next);
	}, []);
	const saveScope = useCallback(
		(scope: GraphOptions) => {
			graphOptionsRef.current = scope;
			setGraphOptions(scope);
			const scopedCase = {
				...caseFileRef.current,
				scope,
				updatedAt: new Date().toISOString(),
			};
			updateCaseFile(scopedCase);
			saveLocalCase(scopedCase);
		},
		[updateCaseFile],
	);

	useEffect(() => {
		const storedCase = loadLocalCase();
		const params = new URLSearchParams(window.location.search);
		const queryNetwork = params.get('network');
		const network = isNetworkSlug(queryNetwork)
			? networkFromSlug(queryNetwork)
			: networkFromSlug(storedCase.network);
		const scope = scopeFromQuery(params, storedCase.scope);
		const address = params.get('address')?.trim() ?? '';
		const initialCase = {
			...storedCase,
			network: networkDetails(network).slug,
			scope,
			rootAddress: address || storedCase.rootAddress,
		};
		updateCaseFile(initialCase);
		graphOptionsRef.current = scope;
		setGraphOptions(scope);
		setNetwork(network);
		setAddress(address);
		if (address) initialSearchRef.current = { address, network, scope };
		setCaseLoaded(true);
	}, [updateCaseFile]);

	useEffect(() => {
		let active = true;
		void fetchNetworkCapabilities()
			.then((capabilities) => {
				if (active) setNetworkCapabilities(capabilities);
			})
			.catch((error) => console.error('Unable to load network capabilities', error));
		return () => {
			active = false;
		};
	}, []);

	useEffect(() => {
		if (caseLoaded) saveLocalCase(caseFile);
	}, [caseFile, caseLoaded]);

	const load = useCallback(
		async (
			target: string,
			preserveCurrentGraph = false,
			targetNetwork = network,
			options = graphOptionsRef.current,
		) => {
			const investigation = preserveCurrentGraph
				? investigationRef.current
				: ++investigationRef.current;
			if (!preserveCurrentGraph) setLoading(true);
			setErrorMessage('');
			if (!preserveCurrentGraph) {
				selectionRef.current++;
				setGraphData(null);
				setSelectedNode(null);
				setSelectedEdge(null);
				setSelectedRelationship([]);
				setSelectedLead(null);
				setHighlightedTransferIds([]);
				setSummary(null);
				setLabels([]);
				setFieldStatuses([]);
				setBranchPages({});
				setExpandingAddress(null);
				setPendingExpansion(null);
				setFrozenEvidence(null);
			}
			try {
				const graph = preserveCurrentGraph
					? await fetchTraceStatus(target, targetNetwork, '', options)
					: await fetchTraceGraph(target, targetNetwork, options, true);
				if (investigation !== investigationRef.current) return;
				setErrorMessage('');
				setGraphData(graph);
				setSelectedNode(graph.nodes.find((node) => node.isSeed) ?? null);
				updateCaseFile((current) => ({
					...current,
					network: networkDetails(targetNetwork).slug,
					scope: options,
					rootAddress: graph.seedAddress,
					updatedAt: new Date().toISOString(),
				}));
				setBranchPages({
					[graph.seedAddress]: { cursor: graph.nextCursor, hasMore: graph.hasMore },
				});
				if (!preserveCurrentGraph)
					void lookupAddress(target, targetNetwork).then((lookup) => {
						if (investigation !== investigationRef.current) return;
						setSummary(lookup.summary ?? null);
						setLabels(lookup.labels);
						setFieldStatuses(lookup.fieldStatuses);
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
		[network, updateCaseFile],
	);

	useEffect(() => {
		const initial = initialSearchRef.current;
		if (!caseLoaded || !initial) return;
		initialSearchRef.current = null;
		void load(initial.address, false, initial.network, initial.scope);
	}, [caseLoaded, load]);

	useEffect(() => {
		if (!caseLoaded) return;
		const url = new URL(window.location.href);
		const params = url.searchParams;
		params.set('network', networkDetails(network).slug);
		if (address) params.set('address', address);
		else params.delete('address');
		params.set('counterparties', String(graphOptions.maxCounterparties));
		params.set('ranking', String(graphOptions.ranking));
		params.set('direction', String(graphOptions.direction));
		params.set('depth', String(graphOptions.maxDepth));
		for (const key of ['from', 'to', 'asset', 'minimumAmount', 'transferKind'] as const) {
			if (graphOptions[key]) params.set(key, graphOptions[key]);
			else params.delete(key);
		}
		window.history.replaceState({}, '', `${url.pathname}?${params.toString()}${url.hash}`);
	}, [address, caseLoaded, graphOptions, network]);

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
			setFieldStatuses([]);
			setBranchPages({});
			setExpandingAddress(null);
			setPendingExpansion(null);
			setFrozenEvidence(null);
			setErrorMessage('');
			updateCaseFile((current) => ({
				...current,
				network: networkDetails(nextNetwork).slug,
				rootAddress: '',
				selectedAddressIds: [],
				selectedTransferIds: [],
				pinnedBridgeTransitionIds: [],
				annotations: [],
				updatedAt: new Date().toISOString(),
			}));
		},
		[network, updateCaseFile],
	);

	useEffect(() => {
		if (!graphData?.pending) return;
		const timer = window.setTimeout(
			() => void load(graphData.seedAddress, true, network, graphOptions),
			tracePollInterval,
		);
		return () => window.clearTimeout(timer);
	}, [graphData, graphOptions, load, network]);

	const handleSelect = useCallback(
		async (node: GraphNode | null) => {
			const selection = ++selectionRef.current;
			setInspectorTab('address');
			setSelectedNode(node);
			setSelectedEdge(null);
			setSelectedRelationship([]);
			setSelectedLead(null);
			setHighlightedTransferIds([]);
			if (node)
				updateCaseFile((current) => ({
					...current,
					selectedAddressIds: [...new Set([...current.selectedAddressIds, node.id])],
					updatedAt: new Date().toISOString(),
				}));
			if (!node) return;
			try {
				const lookup = await lookupAddress(node.id, network);
				if (selection !== selectionRef.current) return;
				setSummary(lookup.summary ?? null);
				setLabels(lookup.labels);
				setFieldStatuses(lookup.fieldStatuses);
			} catch (error) {
				console.error(error);
			}
		},
		[network, updateCaseFile],
	);

	const handleExpand = useCallback(
		async (nodeAddress: string, retry = true) => {
			const key = nodeAddress;
			const page = branchPages[key];
			if (
				!graphData ||
				(nodeDepths(graphData.seedAddress, graphData.edges).get(nodeAddress) ?? 0) >=
					graphOptions.maxDepth ||
				expandingAddress ||
				(pendingExpansion && pendingExpansion !== key) ||
				(page && !page.hasMore)
			)
				return;
			const investigation = investigationRef.current;
			setExpandingAddress(key);
			try {
				const expanded = retry
					? await expandNode(nodeAddress, network, page?.cursor, graphOptions, true)
					: await fetchTraceStatus(nodeAddress, network, page?.cursor, graphOptions).then(
							(status) => ({
								newNodes: status.nodes,
								newEdges: status.edges,
								leads: status.leads,
								crossChainTransitions: status.crossChainTransitions,
								nextCursor: status.nextCursor,
								hasMore: status.hasMore,
								pending: status.pending,
							}),
						);
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
					const transitionIDs = new Set(
						current.crossChainTransitions.map((transition) => transition.id),
					);
					const crossChainTransitions = [
						...current.crossChainTransitions,
						...expanded.crossChainTransitions.filter(
							(transition) => !transitionIDs.has(transition.id),
						),
					];
					return new TraceGraphResponse({
						...current,
						nodes,
						edges,
						leads,
						crossChainTransitions,
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
		[branchPages, expandingAddress, graphData, graphOptions, network, pendingExpansion],
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
	const activeBranch = branchPages[activeAddress];
	const activeDepth = graphData
		? (nodeDepths(graphData.seedAddress, graphData.edges).get(activeAddress) ?? 0)
		: 0;
	const canExpand =
		Boolean(activeAddress) &&
		activeDepth < graphOptions.maxDepth &&
		(!activeBranch || activeBranch.hasMore);
	const selectTransfer = useCallback(
		(edge: GraphEdge) => {
			setInspectorTab('address');
			setSelectedEdge(edge);
			setSelectedLead(null);
			setHighlightedTransferIds([edge.id]);
			updateCaseFile((current) => ({
				...current,
				selectedTransferIds: [...new Set([...current.selectedTransferIds, edge.id])],
				updatedAt: new Date().toISOString(),
			}));
		},
		[updateCaseFile],
	);
	const selectLead = useCallback(
		(lead: InvestigationLead) => {
			if (!graphData) return;
			const evidence = graphData.edges.filter((edge) => lead.transferIds.includes(edge.id));
			const subject = graphData.nodes.find((node) => node.id === lead.subjectAddress) ?? null;
			const selection = ++selectionRef.current;
			setInspectorTab('findings');
			setSelectedLead(lead);
			setSelectedRelationship(evidence);
			setSelectedEdge(evidence[0] ?? null);
			setSelectedNode(subject);
			setHighlightedTransferIds(lead.transferIds);
			updateCaseFile((current) => ({
				...current,
				selectedAddressIds: subject
					? [...new Set([...current.selectedAddressIds, subject.id])]
					: current.selectedAddressIds,
				selectedTransferIds: [...new Set([...current.selectedTransferIds, ...lead.transferIds])],
				updatedAt: new Date().toISOString(),
			}));
			if (subject)
				void lookupAddress(subject.id, network).then((lookup) => {
					if (selection !== selectionRef.current) return;
					setSummary(lookup.summary ?? null);
					setLabels(lookup.labels);
					setFieldStatuses(lookup.fieldStatuses);
				});
		},
		[graphData, network, updateCaseFile],
	);
	const exportFrozenEvidence = useCallback(async () => {
		if (!graphData) throw new Error('Trace an address before creating an evidence package.');
		const evidenceIDs = new Set([
			...graphData.edges.map((edge) => edge.id),
			...graphData.leads.flatMap((lead) => lead.transferIds),
		]);
		const evidenceCase = { ...caseFileRef.current, scope: graphOptionsRef.current };
		const pinnedBridgeSourceIDs = graphData.crossChainTransitions
			.filter((transition) => evidenceCase.pinnedBridgeTransitionIds.includes(transition.id))
			.map((transition) => transition.sourceTransferId)
			.filter((id) => evidenceIDs.has(id));
		const selected = [
			...new Set([...evidenceCase.selectedTransferIds, ...pinnedBridgeSourceIDs]),
		].filter((id) => evidenceIDs.has(id));
		const transferIDs = selected.length > 0 ? selected : [...evidenceIDs];
		if (transferIDs.length === 0) throw new Error('This investigation has no transfers to export.');
		const packageJSON = await exportEvidencePackage(
			network,
			transferIDs,
			JSON.stringify(evidenceCase),
		);
		const packageFile = await parseEvidencePackage(packageJSON);
		setFrozenEvidence(packageFile);
		return packageJSON;
	}, [graphData, network]);
	const importFrozenEvidence = useCallback(
		(packageFile: EvidencePackage) => {
			const replay = replayEvidencePackage(packageFile);
			investigationRef.current++;
			setAddress(replay.caseFile.rootAddress);
			setNetwork(replay.network);
			graphOptionsRef.current = replay.caseFile.scope;
			setGraphOptions(replay.caseFile.scope);
			updateCaseFile(replay.caseFile);
			setGraphData(replay.graph);
			setSelectedNode(replay.graph.nodes.find((node) => node.isSeed) ?? null);
			setSelectedEdge(null);
			setSelectedRelationship([]);
			setSelectedLead(null);
			setHighlightedTransferIds([]);
			setSummary(null);
			setLabels([]);
			setFieldStatuses([]);
			setBranchPages({});
			setExpandingAddress(null);
			setPendingExpansion(null);
			setErrorMessage('');
			setFrozenEvidence(packageFile);
		},
		[updateCaseFile],
	);
	const traceDirection = useCallback(
		(direction: TraceDirection) => {
			if (!graphData) return;
			const scope = { ...graphOptionsRef.current, direction };
			saveScope(scope);
			void load(graphData.seedAddress, false, network, scope);
		},
		[graphData, load, network, saveScope],
	);
	const toggleEvidencePin = useCallback(
		(transferID: string) => {
			updateCaseFile((current) => ({
				...current,
				selectedTransferIds: current.selectedTransferIds.includes(transferID)
					? current.selectedTransferIds.filter((id) => id !== transferID)
					: [...current.selectedTransferIds, transferID],
				updatedAt: new Date().toISOString(),
			}));
		},
		[updateCaseFile],
	);
	const toggleBridgePathPin = useCallback(
		(transitionID: string) => {
			updateCaseFile((current) => ({
				...current,
				pinnedBridgeTransitionIds: current.pinnedBridgeTransitionIds.includes(transitionID)
					? current.pinnedBridgeTransitionIds.filter((id) => id !== transitionID)
					: [...current.pinnedBridgeTransitionIds, transitionID],
				updatedAt: new Date().toISOString(),
			}));
		},
		[updateCaseFile],
	);

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
					loading={loading}
				/>
			)}
			<main
				id="investigation-workspace"
				className="min-h-0 flex-1 flex flex-col md:flex-row overflow-hidden"
			>
				<h1 className="sr-only">OpenChain investigation workspace</h1>
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
						onTraceDirection={traceDirection}
						canExpand={canExpand}
						expanding={expandingAddress === activeAddress || pendingExpansion === activeAddress}
						graphOptions={graphOptions}
						onGraphOptionsChange={(options) => {
							const requiresNewTrace =
								options.direction !== graphOptions.direction ||
								options.maxCounterparties !== graphOptions.maxCounterparties ||
								options.ranking !== graphOptions.ranking;
							saveScope(options);
							if (graphData && requiresNewTrace)
								void load(graphData.seedAddress, false, network, options);
						}}
					/>
					{(errorMessage || graphData?.sourceStatus?.warning) && (
						<div
							className="absolute bottom-5 right-5 z-10 max-w-md space-y-2 text-xs pointer-events-none"
							aria-live="polite"
						>
							{errorMessage && (
								<div
									role="alert"
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
					className="w-full max-h-[42dvh] md:max-h-none md:w-[clamp(18rem,22vw,22rem)] shrink-0 overflow-y-auto p-4"
					style={{ borderLeft: '1px solid var(--border)', background: 'rgba(255,255,255,0.70)' }}
				>
					<div
						className="sticky top-0 z-20 -mx-4 mb-2 px-4 pb-2 pt-0"
						style={{ background: 'rgba(255,255,255,0.96)' }}
					>
						<h3
							className="text-[10px] uppercase font-bold tracking-widest mb-2"
							style={{ color: 'var(--ink-3)' }}
						>
							Address Inspector
						</h3>
						<div
							className="flex rounded-lg p-1"
							role="tablist"
							aria-label="Inspector sections"
							style={{ background: 'var(--slate)', border: '1px solid var(--border)' }}
						>
							{(
								[
									['address', 'Address'],
									[
										'findings',
										`Findings${graphData?.leads.length ? ` · ${graphData.leads.length}` : ''}`,
									],
									[
										'evidence',
										`Evidence${graphData?.edges.length ? ` · ${graphData.edges.length}` : ''}`,
									],
									['case', 'Case'],
								] as const
							).map(([tab, label]) => (
								<button
									key={tab}
									type="button"
									role="tab"
									id={`inspector-tab-${tab}`}
									aria-selected={inspectorTab === tab}
									aria-controls="inspector-panel"
									onClick={() => setInspectorTab(tab)}
									className="min-w-0 flex-1 rounded-md px-1 py-1.5 text-[9px] font-semibold"
									style={{
										background: inspectorTab === tab ? 'var(--white)' : 'transparent',
										color: inspectorTab === tab ? 'var(--accent)' : 'var(--ink-2)',
									}}
								>
									{label}
								</button>
							))}
						</div>
					</div>
					<div
						id="inspector-panel"
						role="tabpanel"
						aria-labelledby={`inspector-tab-${inspectorTab}`}
					>
						{inspectorTab === 'address' && (
							<>
								<WalletLookup
									summary={summary}
									labels={labels}
									fieldStatuses={fieldStatuses}
									loading={loading}
									network={network}
									capabilities={networkCapabilities[networkDetails(network).slug]}
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
							</>
						)}
						{inspectorTab === 'findings' && graphData && (
							<InvestigationLeads
								leads={graphData.leads}
								edges={graphData.edges}
								selectedLeadId={selectedLead?.id}
								onSelect={selectLead}
							/>
						)}
						{inspectorTab === 'evidence' && graphData && (
							<>
								<EvidencePaths
									nodes={graphData.nodes}
									edges={graphData.edges}
									network={network}
									pinnedTransferIds={caseFile.selectedTransferIds}
									coverage={graphData.coverage}
									sourceName={graphData.sourceStatus?.source}
									onTogglePin={(transferID) => {
										setInspectorTab('evidence');
										toggleEvidencePin(transferID);
									}}
								/>
								<CrossChainPaths
									transitions={graphData.crossChainTransitions}
									pinnedTransitionIds={caseFile.pinnedBridgeTransitionIds}
									onTogglePin={toggleBridgePathPin}
									onTraceDestination={(transition) => {
										const destination = transition.destinationNetwork as SupportedNetwork;
										if (!transition.recipient) return;
										changeNetwork(destination);
										setAddress(transition.recipient);
										void load(transition.recipient, false, destination, graphOptionsRef.current);
									}}
								/>
							</>
						)}
						{inspectorTab === 'case' && caseLoaded && (
							<CaseWorkspace
								caseFile={caseFile}
								onChange={updateCaseFile}
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
			</main>
		</div>
	);
}
