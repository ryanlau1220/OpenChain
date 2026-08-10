import cytoscape from 'cytoscape';
import {
	ArrowLeftRight,
	Layers,
	Maximize2,
	PlusCircle,
	RotateCcw,
	ZoomIn,
	ZoomOut,
} from 'lucide-react';
import type React from 'react';
import { useEffect, useRef, useState } from 'react';
import {
	EntityType,
	type GraphEdge,
	type GraphNode,
	type TraceGraphResponse,
	entityLabel,
} from '../services/api';

interface GraphCanvasProps {
	graphData: TraceGraphResponse | null;
	selectedNode?: GraphNode | null;
	onNodeSelect: (node: GraphNode | null) => void;
	onEdgeSelect?: (edge: GraphEdge | null) => void;
	onExpandNode?: (address: string) => void;
	canExpand?: boolean;
	expanding?: boolean;
}

// PRISM node palette (light mode)
const NODE_COLORS = {
	seed: { bg: '#887DFF', border: '#A7F9FF', text: '#fff' },
	exchange: { bg: '#F59E0B', border: '#FDE68A', text: '#fff' },
	contract: { bg: '#8B5CF6', border: '#DDD6FE', text: '#fff' },
	default: { bg: '#4B5068', border: '#C8CADC', text: '#fff' },
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

export const GraphCanvas: React.FC<GraphCanvasProps> = ({
	graphData,
	selectedNode: propSelectedNode,
	onNodeSelect,
	onEdgeSelect,
	onExpandNode,
	canExpand = false,
	expanding = false,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const cyRef = useRef<cytoscape.Core | null>(null);
	const [internalSelectedNode, setInternalSelectedNode] = useState<GraphNode | null>(null);
	const [_selectedEdge, setSelectedEdge] = useState<GraphEdge | null>(null);
	const [layoutName, setLayoutName] = useState<'cose' | 'concentric' | 'breadthfirst' | 'grid'>(
		'breadthfirst',
	);

	const selectedNode = propSelectedNode !== undefined ? propSelectedNode : internalSelectedNode;

	const onNodeSelectRef = useRef(onNodeSelect);
	useEffect(() => {
		onNodeSelectRef.current = onNodeSelect;
	}, [onNodeSelect]);
	const onEdgeSelectRef = useRef(onEdgeSelect);
	useEffect(() => {
		onEdgeSelectRef.current = onEdgeSelect;
	}, [onEdgeSelect]);

	useEffect(() => {
		if (!cyRef.current) return;
		applyNodeStyles(cyRef.current, selectedNode?.id);
	}, [selectedNode?.id, graphData]);

	useEffect(() => {
		if (!containerRef.current || !graphData) return;

		const elements: cytoscape.ElementDefinition[] = [];

		(graphData?.nodes || []).forEach((n) => {
			let palette = NODE_COLORS.default;
			let badge = entityLabel(n.entityType);

			if (n.isSeed) {
				palette = NODE_COLORS.seed;
				badge = 'Target';
			} else if (n.entityType === EntityType.EXCHANGE) {
				palette = NODE_COLORS.exchange;
				badge = 'Exchange';
			} else if (n.entityType === EntityType.CONTRACT) {
				palette = NODE_COLORS.contract;
				badge = 'Contract';
			}

			const inCount = n.inTxCount ?? 0;
			const outCount = n.outTxCount ?? 0;
			const displayLabel = `${outCount}↑ ${inCount}↓\n${n.label || n.id.substring(0, 8)}`;

			elements.push({
				group: 'nodes',
				data: {
					id: n.id,
					label: displayLabel,
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

		(graphData?.edges || []).forEach((e) => {
			const isStable = e.asset?.symbol === 'USDT' || e.asset?.symbol === 'USDC';
			const lineColor = isStable ? '#34D399' : '#887DFF';
			elements.push({
				group: 'edges',
				data: {
					id: e.id,
					source: e.source,
					target: e.target,
					label: `${e.txCount} Tx`,
					color: lineColor,
					raw: e,
				},
			});
		});

		const effectiveLayout =
			layoutName === 'breadthfirst' && (graphData?.edges || []).length === 0 ? 'grid' : layoutName;

		if (cyRef.current) {
			const cy = cyRef.current;
			const currentElementIds = new Set(cy.elements().map((e) => e.id()));
			const newElements = elements.filter(
				(e) => Boolean(e.data.id) && !currentElementIds.has(e.data.id as string),
			);

			if (newElements.length === 0) {
				applyNodeStyles(cy, selectedNode?.id);
				return;
			}

			if (newElements.length > 0 && cy.elements().length > 0) {
				cy.batch(() => {
					cy.add(newElements);
				});
				applyNodeStyles(cy, selectedNode?.id);
				const layout = cy.layout({
					name: effectiveLayout,
					fit: false,
					animate: true,
				});
				layout.run();
				applyNodeStyles(cy, selectedNode?.id);
				return;
			}

			cy.batch(() => {
				cy.elements().remove();
				cy.add(elements);
			});
			applyNodeStyles(cy, selectedNode?.id);
			const layout = cy.layout({
				name: effectiveLayout,
				fit: true,
				padding: 80,
				animate: true,
			});
			layout.run();
			applyNodeStyles(cy, selectedNode?.id);
			if (cy.nodes().length > 0) {
				cy.fit(cy.nodes(), 100);
				cy.center(cy.nodes());
			}
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
						'font-size': '10px',
						'font-family': 'Inter, sans-serif',
						'text-valign': 'top',
						'text-margin-y': -8,
						'text-wrap': 'wrap',
						'text-max-width': '120px',
						'text-outline-width': 2,
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
						'font-size': '9px',
						color: '#4B5068',
						'text-background-opacity': 1,
						'text-background-color': '#FFFFFF',
						'text-background-padding': '2px',
						'text-background-shape': 'roundrectangle',
						'text-border-opacity': 1,
						'text-border-width': 1,
						'text-border-color': '#E6E8EF',
						opacity: 0.85,
					},
				},
			],
			layout: {
				name: effectiveLayout,
				directed: true,
				padding: 60,
				animate: true,
			},
		});

		if (cy.nodes().length > 0) {
			cy.fit(cy.nodes(), 100);
			cy.center(cy.nodes());
		}

		applyNodeStyles(cy, selectedNode?.id);

		cy.minZoom(0.3);
		cy.maxZoom(2.0);

		let isPanningCanvas = false;
		cy.on('pan', () => {
			isPanningCanvas = true;
		});
		cy.on('vmousedown', () => {
			isPanningCanvas = false;
		});

		cy.on('tap select click', 'node', (evt) => {
			const nData = evt.target.data('raw');
			if (!nData) return;
			setInternalSelectedNode(nData);
			setSelectedEdge(null);
			onEdgeSelectRef.current?.(null);
			onNodeSelectRef.current(nData);
			applyNodeStyles(cy, nData.id);
		});

		cy.on('tap', 'edge', (evt) => {
			const edge = evt.target.data('raw') as GraphEdge;
			setSelectedEdge(edge);
			onEdgeSelectRef.current?.(edge);
		});

		cy.on('tap', (evt) => {
			if (evt.target === cy && !isPanningCanvas) {
				setInternalSelectedNode(null);
				setSelectedEdge(null);
				onEdgeSelectRef.current?.(null);
				onNodeSelectRef.current(null);
				applyNodeStyles(cy, undefined);
			}
		});

		cyRef.current = cy;
		return () => {
			cy.destroy();
			cyRef.current = null;
		};
	}, [graphData, layoutName]);

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
					<select
						value={layoutName}
						onChange={(e) =>
							setLayoutName(e.target.value as 'cose' | 'concentric' | 'breadthfirst' | 'grid')
						}
						className="text-xs rounded-lg px-2.5 py-1 focus:outline-none"
						style={{
							background: 'var(--slate)',
							border: '1px solid var(--border)',
							color: 'var(--ink-2)',
						}}
					>
						<option value="breadthfirst">Flow (L→R)</option>
						<option value="cose">Force Directed</option>
						<option value="concentric">Concentric</option>
						<option value="grid">Grid</option>
					</select>
				</div>
			</div>

			{/* Cytoscape canvas */}
			<div ref={containerRef} className="flex-1 w-full h-full cursor-grab active:cursor-grabbing" />

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
			{(!graphData || (graphData.nodes || []).length === 0) && (
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
						Enter a target address above to start tracing
					</p>
				</div>
			)}
		</div>
	);
};
