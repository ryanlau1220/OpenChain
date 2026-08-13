import cytoscape from 'cytoscape';
import {
	ArrowLeftRight,
	Filter,
	Layers,
	LoaderCircle,
	Maximize2,
	PlusCircle,
	RotateCcw,
	ZoomIn,
	ZoomOut,
} from 'lucide-react';
import type React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import {
	EntityType,
	type GraphEdge,
	type GraphNode,
	type GraphOptions,
	GraphRanking,
	TraceDirection,
	type TraceGraphResponse,
	entityLabel,
} from '../services/api';

interface GraphCanvasProps {
	graphData: TraceGraphResponse | null;
	selectedNode?: GraphNode | null;
	onNodeSelect: (node: GraphNode | null) => void;
	onEdgeSelect?: (edge: GraphEdge | null) => void;
	onRelationshipSelect?: (edges: readonly GraphEdge[]) => void;
	onExpandNode?: (address: string) => void;
	canExpand?: boolean;
	expanding?: boolean;
	highlightedTransferIds?: readonly string[];
	loading?: boolean;
	emptyMessage?: string;
	graphOptions?: GraphOptions;
	onGraphOptionsChange?: (options: GraphOptions) => void;
}

// PRISM node palette (light mode)
const NODE_COLORS = {
	seed: { bg: '#887DFF', border: '#A7F9FF', text: '#fff' },
	service: { bg: '#0F766E', border: '#99F6E4', text: '#fff' },
	exchange: { bg: '#F59E0B', border: '#FDE68A', text: '#fff' },
	contract: { bg: '#8B5CF6', border: '#DDD6FE', text: '#fff' },
	default: { bg: '#4B5068', border: '#C8CADC', text: '#fff' },
};

export type GraphRelationship = {
	id: string;
	source: string;
	target: string;
	label: string;
	timeRange: string;
	color: string;
	representative: GraphEdge;
	transfers: GraphEdge[];
	provisional: boolean;
};

type DirectionFilter = 'both' | 'inbound' | 'outbound';
type GraphFilters = {
	from: string;
	to: string;
	direction: DirectionFilter;
	asset: string;
	minimumAmount: string;
	transferKind: string;
};

const emptyFilters: GraphFilters = {
	from: '',
	to: '',
	direction: 'both',
	asset: '',
	minimumAmount: '',
	transferKind: '',
};
const emptyTransferIDs: readonly string[] = [];

type PositionedNode = { id: string; x: number; y: number };
type LayoutName = 'flow' | 'cose' | 'concentric' | 'grid';
type NodeTransferCounts = { inbound: number; outbound: number };

const shortAddress = (address: string) =>
	address.length > 14 ? `${address.slice(0, 6)}…${address.slice(-4)}` : address;

const formatTransferTime = (timestamp: bigint) => {
	const date = new Date(Number(timestamp) * 1000);
	if (!Number.isFinite(date.getTime())) return 'Unknown time';
	return new Intl.DateTimeFormat(undefined, {
		month: 'short',
		day: 'numeric',
		timeZone: 'UTC',
	}).format(date);
};

export function formatRelationshipTimeRange(first: bigint, last: bigint): string {
	const start = formatTransferTime(first);
	const end = formatTransferTime(last);
	return start === end ? start : `${start}–${end}`;
}

export function transferCountsByNode(edges: readonly GraphEdge[]): Map<string, NodeTransferCounts> {
	const counts = new Map<string, NodeTransferCounts>();
	for (const edge of edges) {
		const quantity = edge.txCount || 1;
		const source = counts.get(edge.source) || { inbound: 0, outbound: 0 };
		const target = counts.get(edge.target) || { inbound: 0, outbound: 0 };
		source.outbound += quantity;
		target.inbound += quantity;
		counts.set(edge.source, source);
		counts.set(edge.target, target);
	}
	return counts;
}

const nodeBadge = (node: GraphNode) => {
	if (node.isSeed) return 'TARGET';
	if (node.labels.length > 0) return 'LABELLED SERVICE';
	if (node.entityType === EntityType.EOA || node.entityType === EntityType.UNSPECIFIED)
		return 'UNKNOWN ADDRESS';
	return entityLabel(node.entityType);
};

const nodePalette = (node: GraphNode) => {
	if (node.isSeed) return NODE_COLORS.seed;
	if (node.labels.length > 0) return NODE_COLORS.service;
	if (node.entityType === EntityType.EXCHANGE) return NODE_COLORS.exchange;
	if (node.entityType === EntityType.CONTRACT) return NODE_COLORS.contract;
	return NODE_COLORS.default;
};

// Existing positions are deliberately immutable during an incremental expand.
// New neighbours fan out from their already-rendered parent.
export function positionAddedNodes(
	addedIDs: readonly string[],
	relationships: readonly { source: string; target: string }[],
	existing: readonly PositionedNode[],
	flow = false,
): Map<string, { x: number; y: number }> {
	const positions = new Map(existing.map((node) => [node.id, { x: node.x, y: node.y }]));
	const pending = new Set(addedIDs);
	const grouped = new Map<string, string[]>();
	for (const id of addedIDs) {
		const edge = relationships.find(
			(item) =>
				(item.source === id && positions.has(item.target)) ||
				(item.target === id && positions.has(item.source)),
		);
		const anchor = edge ? (edge.source === id ? edge.target : edge.source) : '';
		const nodes = grouped.get(anchor) || [];
		nodes.push(id);
		grouped.set(anchor, nodes);
	}
	for (const [anchor, ids] of grouped) {
		const center = positions.get(anchor) || { x: 0, y: 0 };
		ids.sort().forEach((id, index) => {
			if (flow) {
				const edge = relationships.find(
					(item) =>
						(item.source === id && item.target === anchor) ||
						(item.target === id && item.source === anchor),
				);
				const isInbound = edge?.target === anchor;
				positions.set(id, {
					x: center.x + (isInbound ? -220 : 220),
					y: center.y + (index - (ids.length - 1) / 2) * 110,
				});
				pending.delete(id);
				return;
			}
			const angle = (Math.PI * 2 * index) / ids.length - Math.PI / 2;
			const radius = anchor ? 130 : 180;
			positions.set(id, {
				x: center.x + Math.cos(angle) * radius,
				y: center.y + Math.sin(angle) * radius,
			});
			pending.delete(id);
		});
	}
	for (const id of pending) positions.set(id, { x: 180, y: 0 });
	return positions;
}

// Source addresses remain left of the subject, destinations right. This keeps
// the graph readable as branches grow without relying on a random simulation.
export function flowNodePositions(
	seedAddress: string,
	nodeIDs: readonly string[],
	relationships: readonly { source: string; target: string }[],
	direction: TraceDirection,
): Map<string, { x: number; y: number }> {
	const inbound = new Map<string, string[]>();
	const outbound = new Map<string, string[]>();
	for (const relationship of relationships) {
		inbound.set(relationship.target, [
			...(inbound.get(relationship.target) || []),
			relationship.source,
		]);
		outbound.set(relationship.source, [
			...(outbound.get(relationship.source) || []),
			relationship.target,
		]);
	}
	const distancesFrom = (next: Map<string, string[]>) => {
		const distances = new Map<string, number>([[seedAddress, 0]]);
		const queue = [seedAddress];
		for (let index = 0; index < queue.length; index++) {
			const current = queue[index];
			for (const neighbour of next.get(current) || []) {
				if (distances.has(neighbour)) continue;
				distances.set(neighbour, (distances.get(current) || 0) + 1);
				queue.push(neighbour);
			}
		}
		return distances;
	};
	const inboundDepth = distancesFrom(inbound);
	const outboundDepth = distancesFrom(outbound);
	const columns = new Map<number, string[]>();
	for (const id of nodeIDs) {
		let column = 0;
		if (id !== seedAddress) {
			const inDepth = inboundDepth.get(id);
			const outDepth = outboundDepth.get(id);
			if (direction === TraceDirection.INBOUND) column = inDepth ? -inDepth : 1;
			else if (direction === TraceDirection.OUTBOUND) column = outDepth || -1;
			else if (inDepth && !outDepth) column = -inDepth;
			else if (outDepth && !inDepth) column = outDepth;
			else if (inDepth && outDepth) column = outDepth <= inDepth ? outDepth : -inDepth;
			else column = 1;
		}
		columns.set(column, [...(columns.get(column) || []), id]);
	}
	const positions = new Map<string, { x: number; y: number }>();
	for (const [column, ids] of columns) {
		ids.sort().forEach((id, index) => {
			positions.set(id, {
				x: column * 220,
				y: (index - (ids.length - 1) / 2) * 110,
			});
		});
	}
	return positions;
}

const baseUnits = (value: string) => {
	try {
		return BigInt(value);
	} catch {
		return 0n;
	}
};

const formatRelationshipAmount = (amount: bigint, edge: GraphEdge) => {
	const asset = edge.asset;
	if (!asset?.symbol) return `${amount.toString()} units`;
	const decimals = Math.min(asset.decimals, 30);
	const divisor = 10n ** BigInt(decimals);
	const whole = amount / divisor;
	const fraction = (amount % divisor).toString().padStart(decimals, '0').slice(0, 4);
	return `${whole}${fraction.replace(/0+$/, '') ? `.${fraction.replace(/0+$/, '')}` : ''} ${asset.symbol}`;
};

export function aggregateGraphEdges(edges: readonly GraphEdge[]): GraphRelationship[] {
	const relationships = new Map<
		string,
		{ count: number; amount: bigint; representative: GraphEdge; transfers: GraphEdge[] }
	>();
	for (const edge of edges) {
		const asset = edge.asset;
		const assetKey = [asset?.kind, asset?.contractAddress || asset?.symbol, asset?.decimals].join(
			':',
		);
		const id = `relationship:${edge.source}:${edge.target}:${assetKey}`;
		const relationship = relationships.get(id);
		if (!relationship) {
			relationships.set(id, {
				count: edge.txCount || 1,
				amount: baseUnits(edge.amountBaseUnits),
				representative: edge,
				transfers: [edge],
			});
			continue;
		}
		relationship.count += edge.txCount || 1;
		relationship.amount += baseUnits(edge.amountBaseUnits);
		relationship.transfers.push(edge);
		if (Number(edge.lastTxTimestamp) > Number(relationship.representative.lastTxTimestamp))
			relationship.representative = edge;
	}
	return [...relationships].map(([id, relationship]) => {
		const edge = relationship.representative;
		const stable = edge.asset?.symbol === 'USDT' || edge.asset?.symbol === 'USDC';
		const transfers = relationship.transfers.toSorted((left, right) => {
			if (left.firstTxTimestamp !== right.firstTxTimestamp)
				return Number(left.firstTxTimestamp) - Number(right.firstTxTimestamp);
			return left.id.localeCompare(right.id);
		});
		const first = transfers[0];
		const last = transfers[transfers.length - 1];
		return {
			id,
			source: edge.source,
			target: edge.target,
			label: `${relationship.count} ${relationship.count === 1 ? 'transfer' : 'transfers'} · ${formatRelationshipAmount(relationship.amount, edge)}\n${formatRelationshipTimeRange(first.firstTxTimestamp, last.lastTxTimestamp)}`,
			timeRange: formatRelationshipTimeRange(first.firstTxTimestamp, last.lastTxTimestamp),
			color: stable ? '#34D399' : '#887DFF',
			representative: edge,
			transfers,
			provisional: relationship.transfers.some((transfer) => transfer.provisional),
		};
	});
}

const assetKey = (edge: GraphEdge) =>
	[edge.asset?.kind, edge.asset?.contractAddress || edge.asset?.symbol, edge.asset?.decimals].join(
		':',
	);

const minimumBaseUnits = (value: string, decimals: number) => {
	if (!/^\d+(?:\.\d+)?$/.test(value)) return null;
	const [whole, fraction = ''] = value.split('.');
	const safeDecimals = Math.min(decimals, 30);
	if (fraction.length > safeDecimals) return null;
	return BigInt(whole + fraction.padEnd(safeDecimals, '0'));
};

export function filterGraphEdges(
	edges: readonly GraphEdge[],
	seedAddress: string,
	filters: GraphFilters,
): GraphEdge[] {
	const fromTimestamp = filters.from ? Date.parse(filters.from) : 0;
	const toTimestamp = filters.to ? Date.parse(filters.to) : Number.POSITIVE_INFINITY;
	if (Number.isNaN(fromTimestamp) || Number.isNaN(toTimestamp)) return [];
	const from = fromTimestamp / 1000;
	const to =
		toTimestamp === Number.POSITIVE_INFINITY ? toTimestamp : (toTimestamp + 86_399_999) / 1000;
	return edges.filter((edge) => {
		const timestamp = Number(edge.firstTxTimestamp);
		if (timestamp < from || timestamp > to || (filters.asset && assetKey(edge) !== filters.asset))
			return false;
		if (filters.transferKind && edge.transferKind !== filters.transferKind) return false;
		if (filters.direction === 'inbound' && edge.target !== seedAddress) return false;
		if (filters.direction === 'outbound' && edge.source !== seedAddress) return false;
		if (!filters.minimumAmount) return true;
		const minimum = minimumBaseUnits(filters.minimumAmount, edge.asset?.decimals ?? 0);
		return minimum !== null && baseUnits(edge.amountBaseUnits) >= minimum;
	});
}

const applyEvidenceStyles = (cy: cytoscape.Core, transferIDs: ReadonlySet<string>) => {
	cy.edges().forEach((edge) => {
		const relationship = edge.data('raw') as GraphRelationship;
		const highlighted = relationship.transfers.some((transfer) => transferIDs.has(transfer.id));
		edge.toggleClass('evidence-edge', highlighted);
	});
};

const applyNodeStyles = (cy: cytoscape.Core, targetNodeId?: string) => {
	const cleanTarget = targetNodeId?.toLowerCase();
	cy.batch(() => {
		cy.nodes().forEach((n) => {
			const isSeed = n.data('is_seed');
			const palette = n.data('palette') || NODE_COLORS.default;
			const isSelected = Boolean(cleanTarget) && n.id().toLowerCase() === cleanTarget;

			if (isSelected) {
				n.data('borderColor', '#34D399');
				n.data('borderWidth', 6);
				n.addClass('selected-node');
				n.style({
					'border-color': '#34D399',
					'border-width': 6,
					'overlay-opacity': 0,
				});
			} else if (isSeed) {
				n.data('borderColor', '#A7F9FF');
				n.data('borderWidth', 4);
				n.removeClass('selected-node');
				n.style({
					'border-color': '#A7F9FF',
					'border-width': 4,
					'overlay-opacity': 0,
				});
			} else {
				const defColor = palette.border || '#C8CADC';
				n.data('borderColor', defColor);
				n.data('borderWidth', 2);
				n.removeClass('selected-node');
				n.style({
					'border-color': defColor,
					'border-width': 2,
					'overlay-opacity': 0,
				});
			}
		});
	});
};

const layoutAndFit = (cy: cytoscape.Core, name: LayoutName, direction: TraceDirection) => {
	if (name === 'flow') {
		const relationships = cy.edges().map((edge) => ({
			source: edge.source().id(),
			target: edge.target().id(),
		}));
		const seed = cy.nodes().filter('[?is_seed]')[0]?.id() || cy.nodes()[0]?.id() || '';
		const positions = flowNodePositions(
			seed,
			cy.nodes().map((node) => node.id()),
			relationships,
			direction,
		);
		cy.batch(() => {
			cy.nodes().forEach((node) => {
				const position = positions.get(node.id());
				if (position) node.position(position);
			});
		});
		cy.resize();
		if (cy.elements().length > 0) cy.fit(cy.elements(), 100);
		return;
	}
	const layout = cy.layout({ name, directed: true, padding: 80, animate: false });
	layout.one('layoutstop', () => {
		cy.resize();
		if (cy.elements().length > 0) cy.fit(cy.elements(), 100);
	});
	layout.run();
};

export const GraphCanvas: React.FC<GraphCanvasProps> = ({
	graphData,
	selectedNode: propSelectedNode,
	onNodeSelect,
	onEdgeSelect,
	onRelationshipSelect,
	onExpandNode,
	canExpand = false,
	expanding = false,
	highlightedTransferIds = emptyTransferIDs,
	loading = false,
	emptyMessage = 'Enter a target address above to start tracing',
	graphOptions,
	onGraphOptionsChange,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const cyRef = useRef<cytoscape.Core | null>(null);
	const [internalSelectedNode, setInternalSelectedNode] = useState<GraphNode | null>(null);
	const [layoutName, setLayoutName] = useState<LayoutName>('flow');
	const [filters, setFilters] = useState<GraphFilters>(emptyFilters);
	const [filterOpen, setFilterOpen] = useState(false);
	const layoutRef = useRef(`${layoutName}:${graphOptions?.direction ?? TraceDirection.BOTH}`);

	const selectedNode = propSelectedNode !== undefined ? propSelectedNode : internalSelectedNode;
	const seedAddress = graphData?.seedAddress || '';
	const visibleEdges = useMemo(
		() => filterGraphEdges(graphData?.edges || [], seedAddress, filters),
		[filters, graphData?.edges, seedAddress],
	);
	const visibleNodes = useMemo(() => {
		const addresses = new Set([seedAddress]);
		for (const edge of visibleEdges) {
			addresses.add(edge.source);
			addresses.add(edge.target);
		}
		return (graphData?.nodes || []).filter((node) => addresses.has(node.id));
	}, [graphData?.nodes, seedAddress, visibleEdges]);
	const nodeCounts = useMemo(() => transferCountsByNode(visibleEdges), [visibleEdges]);
	const assets = useMemo(() => {
		const unique = new Map<string, string>();
		for (const edge of graphData?.edges || []) {
			const asset = edge.asset;
			unique.set(assetKey(edge), asset?.symbol || 'Unknown asset');
		}
		return [...unique.entries()];
	}, [graphData?.edges]);
	const transferKinds = useMemo(
		() => [...new Set((graphData?.edges || []).map((edge) => edge.transferKind).filter(Boolean))],
		[graphData?.edges],
	);
	const highlightedIDs = useMemo(() => new Set(highlightedTransferIds), [highlightedTransferIds]);

	const onNodeSelectRef = useRef(onNodeSelect);
	useEffect(() => {
		onNodeSelectRef.current = onNodeSelect;
	}, [onNodeSelect]);
	const onEdgeSelectRef = useRef(onEdgeSelect);
	useEffect(() => {
		onEdgeSelectRef.current = onEdgeSelect;
	}, [onEdgeSelect]);
	const onRelationshipSelectRef = useRef(onRelationshipSelect);
	useEffect(() => {
		onRelationshipSelectRef.current = onRelationshipSelect;
	}, [onRelationshipSelect]);

	useEffect(() => {
		if (!cyRef.current) return;
		applyNodeStyles(cyRef.current, selectedNode?.id);
	}, [selectedNode?.id, graphData]);

	useEffect(() => {
		if (cyRef.current) applyEvidenceStyles(cyRef.current, highlightedIDs);
	}, [highlightedIDs]);

	useEffect(
		() => () => {
			cyRef.current?.destroy();
			cyRef.current = null;
		},
		[],
	);

	useEffect(() => {
		if (!containerRef.current || !graphData) return;

		const elements: cytoscape.ElementDefinition[] = [];

		visibleNodes.forEach((n) => {
			const palette = nodePalette(n);
			const badge = nodeBadge(n);
			const counts = nodeCounts.get(n.id) || { inbound: n.inTxCount, outbound: n.outTxCount };
			const displayLabel = n.label || shortAddress(n.id);

			elements.push({
				group: 'nodes',
				data: {
					id: n.id,
					label: `${badge}\n${displayLabel}\n↑ ${counts.inbound} in · ↓ ${counts.outbound} out`,
					badge,
					is_seed: n.isSeed,
					bg: palette.bg,
					borderColor: palette.border,
					borderWidth: n.isSeed ? 4 : 2,
					palette,
					raw: n,
				},
			});
		});

		aggregateGraphEdges(visibleEdges).forEach((relationship) => {
			elements.push({
				group: 'edges',
				data: {
					id: relationship.id,
					source: relationship.source,
					target: relationship.target,
					label: relationship.label,
					color: relationship.color,
					provisional: relationship.provisional,
					raw: relationship,
				},
			});
		});

		const effectiveLayout: LayoutName =
			layoutName === 'flow' && visibleEdges.length === 0 ? 'grid' : layoutName;
		const traceDirection = graphOptions?.direction ?? TraceDirection.BOTH;
		const layoutKey = `${effectiveLayout}:${traceDirection}`;

		if (cyRef.current) {
			const cy = cyRef.current;
			const isInitialHydration =
				cy.nodes().length === 1 && cy.nodes()[0].data('is_seed') && cy.edges().length === 0;
			const currentElementIds = new Set(cy.elements().map((e) => e.id()));
			const desiredElementIds = new Set(elements.map((element) => element.data.id as string));
			const needsRemoval = [...currentElementIds].some((id) => !desiredElementIds.has(id));
			const newElements = elements.filter(
				(element) => !currentElementIds.has(element.data.id as string),
			);
			cy.batch(() => {
				for (const element of elements) {
					const existing = cy.getElementById(element.data.id as string);
					if (existing.length > 0) existing.data(element.data);
				}
			});

			if (newElements.length === 0 && !needsRemoval) {
				if (layoutRef.current !== layoutKey) {
					layoutAndFit(cy, effectiveLayout, traceDirection);
					layoutRef.current = layoutKey;
				}
				applyNodeStyles(cy, selectedNode?.id);
				applyEvidenceStyles(cy, highlightedIDs);
				return;
			}

			if (newElements.length > 0 && !needsRemoval && cy.elements().length > 0) {
				const existing = cy.nodes().map((node) => ({ id: node.id(), ...node.position() }));
				const positions = positionAddedNodes(
					newElements.filter((item) => item.group === 'nodes').map((item) => String(item.data.id)),
					elements
						.filter((item) => item.group === 'edges')
						.map((item) => ({
							source: String(item.data.source),
							target: String(item.data.target),
						})),
					existing,
					effectiveLayout === 'flow',
				);
				for (const element of newElements) {
					if (element.group === 'nodes') element.position = positions.get(String(element.data.id));
				}
				cy.batch(() => {
					cy.add(newElements);
				});
				if (isInitialHydration) layoutAndFit(cy, effectiveLayout, traceDirection);
				applyNodeStyles(cy, selectedNode?.id);
				applyEvidenceStyles(cy, highlightedIDs);
				return;
			}

			cy.batch(() => {
				cy.elements().remove();
				cy.add(elements);
			});
			applyNodeStyles(cy, selectedNode?.id);
			applyEvidenceStyles(cy, highlightedIDs);
			layoutAndFit(cy, effectiveLayout, traceDirection);
			applyNodeStyles(cy, selectedNode?.id);
			return;
		}

		const cy = cytoscape({
			container: containerRef.current,
			elements,
			style: [
				{
					selector: 'node',
					style: {
						'background-color': 'data(bg)',
						label: 'data(label)',
						color: '#FFFFFF',
						'font-size': '9px',
						'font-family': 'Inter, sans-serif',
						'text-valign': 'bottom',
						'text-margin-y': 10,
						'text-wrap': 'wrap',
						'text-max-width': '120px',
						'text-outline-width': 3,
						'text-outline-color': 'data(bg)',
						width: (ele: cytoscape.NodeSingular) => (ele.data('is_seed') ? 52 : 38),
						height: (ele: cytoscape.NodeSingular) => (ele.data('is_seed') ? 52 : 38),
						'border-width': 'data(borderWidth)',
						'border-color': 'data(borderColor)',
						'border-opacity': 1,
						'overlay-padding': '4px',
					},
				},
				{
					selector: 'node[?is_seed]',
					style: {
						width: 52,
						height: 52,
						'border-width': 4,
						'border-color': '#A7F9FF',
					},
				},
				{
					selector: 'node.selected-node',
					style: {
						'border-width': 6,
						'border-color': '#34D399',
						'border-opacity': 1,
					},
				},
				{
					selector: 'edge',
					style: {
						width: 1.5,
						'line-color': 'data(color)',
						'target-arrow-color': 'data(color)',
						'target-arrow-shape': 'triangle',
						'curve-style': 'bezier',
						label: 'data(label)',
						'font-size': '7px',
						color: '#4B5068',
						'text-rotation': 'autorotate',
						'text-margin-y': -12,
						'text-wrap': 'wrap',
						'text-max-width': '110px',
						'text-background-opacity': 1,
						'text-background-color': '#FFFFFF',
						'text-background-padding': '1px',
						'text-background-shape': 'roundrectangle',
						'text-border-opacity': 1,
						'text-border-width': 1,
						'text-border-color': '#E6E8EF',
						opacity: 0.85,
					},
				},
				{
					selector: 'edge.evidence-edge',
					style: {
						width: 4,
						'line-color': '#F59E0B',
						'target-arrow-color': '#F59E0B',
						opacity: 1,
					},
				},
				{
					selector: 'edge[?provisional]',
					style: {
						'line-style': 'dashed',
					},
				},
			],
			layout: { name: 'preset' },
		});
		layoutAndFit(cy, effectiveLayout, traceDirection);

		applyNodeStyles(cy, selectedNode?.id);
		applyEvidenceStyles(cy, highlightedIDs);

		cy.minZoom(0.3);
		cy.maxZoom(2.0);

		let isPanningCanvas = false;
		cy.on('pan', () => {
			isPanningCanvas = true;
		});
		cy.on('vmousedown', () => {
			isPanningCanvas = false;
		});

		cy.on('tap', 'node', (evt) => {
			const nData = evt.target.data('raw');
			if (!nData) return;
			setInternalSelectedNode(nData);
			onEdgeSelectRef.current?.(null);
			onRelationshipSelectRef.current?.([]);
			onNodeSelectRef.current(nData);
			applyNodeStyles(cy, nData.id);
		});

		cy.on('tap', 'edge', (evt) => {
			const relationship = evt.target.data('raw') as GraphRelationship;
			onEdgeSelectRef.current?.(relationship.representative);
			onRelationshipSelectRef.current?.(relationship.transfers);
		});

		cy.on('tap', (evt) => {
			if (evt.target === cy && !isPanningCanvas) {
				setInternalSelectedNode(null);
				onEdgeSelectRef.current?.(null);
				onRelationshipSelectRef.current?.([]);
				onNodeSelectRef.current(null);
				applyNodeStyles(cy, undefined);
			}
		});

		cyRef.current = cy;
		layoutRef.current = layoutKey;
	}, [graphData, graphOptions?.direction, layoutName, nodeCounts, visibleEdges, visibleNodes]);

	const handleZoomIn = () => cyRef.current?.zoom(cyRef.current.zoom() * 1.2);
	const handleZoomOut = () => cyRef.current?.zoom(cyRef.current.zoom() * 0.8);
	const handleFit = () => cyRef.current?.fit();
	const handleReset = () => {
		cyRef.current?.reset();
		cyRef.current?.fit();
	};

	const seedNode = (graphData?.nodes || []).find((n) => n.isSeed);
	const activeTargetAddress = selectedNode?.id || seedNode?.id || graphData?.seedAddress || '';

	const toolBtn =
		'p-1.5 rounded-lg transition hover:bg-[var(--slate)] text-[var(--ink-3)] hover:text-[var(--ink)]';
	const activeFilterCount = Object.values(filters).filter(
		(value) => value && value !== 'both',
	).length;
	const updateFilter = <Key extends keyof GraphFilters>(key: Key, value: GraphFilters[Key]) =>
		setFilters((current) => ({ ...current, [key]: value }));
	const showLoading = loading || Boolean(graphData?.pending);

	return (
		<div className="relative w-full h-full flex flex-col" style={{ background: 'var(--snow)' }}>
			{/* Toolbar */}
			<div
				className="h-11 px-4 flex items-center justify-between text-xs shrink-0"
				style={{
					background: 'rgba(255,255,255,0.85)',
					borderBottom: '1px solid var(--border)',
					backdropFilter: 'blur(8px)',
				}}
			>
				{/* Left: selected + expand */}
				<div className="flex items-center gap-3">
					<div className="flex items-center gap-1.5 font-medium" style={{ color: 'var(--ink-2)' }}>
						<Layers className="w-3.5 h-3.5" style={{ color: 'var(--accent)' }} />
						<span>Selected:</span>
						<span
							className="font-mono px-2 py-0.5 rounded text-[11px]"
							style={{
								background: selectedNode ? 'rgba(52, 211, 153, 0.12)' : 'rgba(136, 125, 255, 0.12)',
								color: selectedNode ? '#059669' : 'var(--accent)',
								border: selectedNode
									? '1px solid rgba(52, 211, 153, 0.3)'
									: '1px solid rgba(136, 125, 255, 0.3)',
							}}
						>
							{selectedNode
								? `${selectedNode.id.substring(0, 6)}…${selectedNode.id.substring(38)}`
								: 'Target Seed'}
						</span>
					</div>

					<div
						className="flex items-center gap-1.5 ml-1 pl-3"
						style={{ borderLeft: '1px solid var(--border)' }}
					>
						<button
							type="button"
							disabled={!activeTargetAddress || !canExpand || expanding}
							onClick={() => activeTargetAddress && onExpandNode?.(activeTargetAddress)}
							className="btn-outline text-[11px] flex items-center gap-1 transition font-medium"
							style={{ padding: '0.25rem 0.625rem' }}
							title={
								selectedNode
									? `Expand counterparty transfers for ${selectedNode.id}`
									: 'Expand transfers'
							}
						>
							<PlusCircle className="w-3.5 h-3.5 text-[var(--accent)]" />
							{expanding ? 'Expanding…' : 'Expand'}
						</button>
					</div>
				</div>
				<div className="flex items-center gap-2">
					{graphOptions && onGraphOptionsChange && (
						<>
							<select
								aria-label="Counterparties per address"
								value={graphOptions.maxCounterparties}
								onChange={(event) =>
									onGraphOptionsChange({
										...graphOptions,
										maxCounterparties: Number(event.target.value),
									})
								}
								className="text-xs rounded-lg px-2.5 py-1 focus:outline-none"
								style={{
									background: 'var(--slate)',
									border: '1px solid var(--border)',
									color: 'var(--ink-2)',
								}}
							>
								<option value={5}>Top 5 counterparties</option>
								<option value={10}>Top 10 counterparties</option>
								<option value={25}>Top 25 counterparties</option>
							</select>
							<select
								aria-label="Counterparty ranking"
								value={graphOptions.ranking}
								onChange={(event) =>
									onGraphOptionsChange({
										...graphOptions,
										ranking: Number(event.target.value) as GraphRanking,
									})
								}
								className="text-xs rounded-lg px-2.5 py-1 focus:outline-none"
								style={{
									background: 'var(--slate)',
									border: '1px solid var(--border)',
									color: 'var(--ink-2)',
								}}
								title="Largest raw amount compares native asset units, not fiat value."
							>
								<option value={GraphRanking.MOST_RECENT}>Most recent</option>
								<option value={GraphRanking.MOST_ACTIVE}>Most active</option>
								<option value={GraphRanking.LARGEST_RAW_AMOUNT}>Largest raw amount</option>
							</select>
							<select
								aria-label="Investigation direction"
								value={graphOptions.direction}
								onChange={(event) =>
									onGraphOptionsChange({
										...graphOptions,
										direction: Number(event.target.value) as TraceDirection,
									})
								}
								className="text-xs rounded-lg px-2.5 py-1 focus:outline-none"
								style={{
									background: 'var(--slate)',
									border: '1px solid var(--border)',
									color: 'var(--ink-2)',
								}}
							>
								<option value={TraceDirection.BOTH}>Full flow</option>
								<option value={TraceDirection.INBOUND}>Source of funds</option>
								<option value={TraceDirection.OUTBOUND}>Destination of funds</option>
							</select>
						</>
					)}
					<button
						type="button"
						aria-expanded={filterOpen}
						onClick={() => setFilterOpen((open) => !open)}
						className="list-none cursor-pointer rounded-lg px-2.5 py-1 text-xs"
						style={{
							background: activeFilterCount ? 'rgba(136,125,255,0.10)' : 'var(--slate)',
							border: '1px solid var(--border)',
							color: activeFilterCount ? 'var(--accent)' : 'var(--ink-2)',
						}}
					>
						<Filter className="mr-1 inline h-3.5 w-3.5" />
						Filters{activeFilterCount ? ` · ${activeFilterCount}` : ''}
					</button>
					<select
						value={layoutName}
						onChange={(e) => setLayoutName(e.target.value as LayoutName)}
						className="text-xs rounded-lg px-2.5 py-1 focus:outline-none"
						style={{
							background: 'var(--slate)',
							border: '1px solid var(--border)',
							color: 'var(--ink-2)',
						}}
					>
						<option value="flow">Flow (source → target → destination)</option>
						<option value="cose">Force Directed</option>
						<option value="concentric">Concentric</option>
						<option value="grid">Grid</option>
					</select>
				</div>
			</div>
			{filterOpen && (
				<div
					className="shrink-0 border-b px-4 py-3"
					style={{ borderColor: 'var(--border)', background: 'var(--white)' }}
				>
					<div className="ml-auto max-w-xl space-y-2">
						<div className="flex items-center justify-between">
							<p
								className="text-[10px] font-semibold uppercase tracking-wider"
								style={{ color: 'var(--ink-3)' }}
							>
								Visible transfers: {visibleEdges.length}/{graphData?.edges.length || 0}
							</p>
							<button
								type="button"
								onClick={() => setFilters(emptyFilters)}
								className="text-[10px]"
								style={{ color: 'var(--accent)' }}
							>
								Clear
							</button>
						</div>
						<div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
							<input
								aria-label="From date"
								type="date"
								value={filters.from}
								onChange={(event) => updateFilter('from', event.target.value)}
								className="prism-input text-[10px] px-2 py-1.5"
							/>
							<input
								aria-label="To date"
								type="date"
								value={filters.to}
								onChange={(event) => updateFilter('to', event.target.value)}
								className="prism-input text-[10px] px-2 py-1.5"
							/>
							<select
								aria-label="Transfer direction"
								value={filters.direction}
								onChange={(event) =>
									updateFilter('direction', event.target.value as DirectionFilter)
								}
								className="prism-input text-[10px] px-2 py-1.5"
							>
								<option value="both">All directions</option>
								<option value="inbound">Inbound to target</option>
								<option value="outbound">Outbound from target</option>
							</select>
							<select
								aria-label="Asset"
								value={filters.asset}
								onChange={(event) => updateFilter('asset', event.target.value)}
								className="prism-input text-[10px] px-2 py-1.5"
							>
								<option value="">All assets</option>
								{assets.map(([id, label]) => (
									<option key={id} value={id}>
										{label}
									</option>
								))}
							</select>
							<input
								aria-label="Minimum amount"
								type="text"
								inputMode="decimal"
								maxLength={30}
								placeholder="Minimum amount"
								value={filters.minimumAmount}
								onChange={(event) => updateFilter('minimumAmount', event.target.value)}
								className="prism-input text-[10px] px-2 py-1.5"
							/>
							<select
								aria-label="Transfer type"
								value={filters.transferKind}
								onChange={(event) => updateFilter('transferKind', event.target.value)}
								className="prism-input text-[10px] px-2 py-1.5"
							>
								<option value="">All types</option>
								{transferKinds.map((kind) => (
									<option key={kind} value={kind}>
										{kind}
									</option>
								))}
							</select>
						</div>
						<p className="text-[9px]" style={{ color: 'var(--ink-3)' }}>
							Amount uses the selected transfer’s asset units.
						</p>
					</div>
				</div>
			)}

			{/* Cytoscape canvas */}
			<div
				ref={containerRef}
				className="min-h-0 flex-1 w-full cursor-grab active:cursor-grabbing"
			/>

			{/* Floating zoom controls */}
			<div
				className="absolute bottom-5 left-5 flex flex-col gap-0.5 p-1 rounded-xl z-10"
				style={{
					background: 'rgba(255,255,255,0.90)',
					border: '1px solid var(--border)',
					boxShadow: '0 4px 16px rgba(26,29,35,0.08)',
					backdropFilter: 'blur(8px)',
				}}
			>
				{[
					{ icon: <ZoomIn className="w-4 h-4" />, action: handleZoomIn, title: 'Zoom In' },
					{ icon: <ZoomOut className="w-4 h-4" />, action: handleZoomOut, title: 'Zoom Out' },
					{ icon: <Maximize2 className="w-4 h-4" />, action: handleFit, title: 'Fit View' },
					{ icon: <RotateCcw className="w-4 h-4" />, action: handleReset, title: 'Reset' },
				].map(({ icon, action, title }) => (
					<button key={title} type="button" onClick={action} className={toolBtn} title={title}>
						{icon}
					</button>
				))}
			</div>

			{/* Empty state */}
			{!showLoading && (!graphData || (graphData.nodes || []).length === 0) && (
				<div
					className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none"
					style={{ background: 'rgba(250,250,252,0.85)' }}
				>
					<div
						className="w-16 h-16 rounded-2xl flex items-center justify-center mb-4"
						style={{
							background: 'linear-gradient(135deg, rgba(136,125,255,0.10), rgba(167,249,255,0.10))',
							border: '1px solid var(--border)',
						}}
					>
						<ArrowLeftRight className="w-7 h-7" style={{ color: 'var(--accent)' }} />
					</div>
					<h3 className="text-sm font-semibold mb-1" style={{ color: 'var(--ink)' }}>
						No Address Flow Rendered
					</h3>
					<p className="text-xs" style={{ color: 'var(--ink-3)' }}>
						{emptyMessage}
					</p>
				</div>
			)}
			{showLoading && (
				<div
					className="absolute inset-0 z-20 grid place-items-center pointer-events-none"
					style={{ background: 'rgba(250,250,252,0.82)' }}
				>
					<div
						className="flex flex-col items-center gap-3 rounded-2xl px-6 py-5"
						style={{ background: 'rgba(255,255,255,0.94)', border: '1px solid var(--border)' }}
					>
						<LoaderCircle className="h-7 w-7 animate-spin" style={{ color: 'var(--accent)' }} />
						<p className="text-sm font-medium" style={{ color: 'var(--ink-2)' }}>
							Retrieving address flow…
						</p>
					</div>
				</div>
			)}
		</div>
	);
};
