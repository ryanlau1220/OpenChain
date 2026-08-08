import cytoscape from 'cytoscape';
import {
	AlertTriangle,
	ArrowLeftRight,
	Download,
	Layers,
	Maximize2,
	MinusCircle,
	PlusCircle,
	Redo2,
	RotateCcw,
	Share2,
	Undo2,
	ZoomIn,
	ZoomOut,
} from 'lucide-react';
import type React from 'react';
import { useEffect, useRef, useState } from 'react';
import {
	EntityType,
	type GraphData,
	type GraphEdge,
	type GraphNode,
	entityLabel,
} from '../services/api';

interface GraphCanvasProps {
	graphData:
		| (GraphData & { sync_state?: { warning_message?: string; is_synced?: boolean } })
		| null;
	onNodeSelect: (node: GraphNode | null) => void;
	onExpandNode?: (address: string) => void;
	onCollapseNode?: (address: string) => void;
	onShareCanvas?: () => void;
	onExportCase?: () => void;
	onUndo?: () => void;
	onRedo?: () => void;
	canUndo?: boolean;
	canRedo?: boolean;
}

// PRISM node palette (light mode)
const NODE_COLORS = {
	seed: { bg: '#887DFF', border: '#A7F9FF', text: '#fff' },
	risk: { bg: '#FF4D4F', border: '#FFB3B3', text: '#fff' },
	exchange: { bg: '#F59E0B', border: '#FDE68A', text: '#fff' },
	contract: { bg: '#8B5CF6', border: '#DDD6FE', text: '#fff' },
	default: { bg: '#4B5068', border: '#C8CADC', text: '#fff' },
};

export const GraphCanvas: React.FC<GraphCanvasProps> = ({
	graphData,
	onNodeSelect,
	onExpandNode,
	onCollapseNode,
	onShareCanvas,
	onExportCase,
	onUndo,
	onRedo,
	canUndo = false,
	canRedo = false,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const cyRef = useRef<cytoscape.Core | null>(null);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
	const [_selectedEdge, setSelectedEdge] = useState<GraphEdge | null>(null);
	const [layoutName, setLayoutName] = useState<'cose' | 'concentric' | 'breadthfirst' | 'grid'>(
		'breadthfirst',
	);

	const onNodeSelectRef = useRef(onNodeSelect);
	useEffect(() => {
		onNodeSelectRef.current = onNodeSelect;
	}, [onNodeSelect]);

	useEffect(() => {
		if (!containerRef.current || !graphData) return;

		const elements: cytoscape.ElementDefinition[] = [];

		(graphData?.nodes || []).forEach((n) => {
			let palette = NODE_COLORS.default;
			let badge = entityLabel(n.entityType);

			if (n.isSeed) {
				palette = NODE_COLORS.seed;
				badge = 'Target';
			} else if (n.category === 'SCAMMER' || n.riskScore >= 50) {
				palette = NODE_COLORS.risk;
				badge = 'High Risk';
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
					raw: n,
				},
			});
		});

		(graphData?.edges || []).forEach((e) => {
			const isStable = e.assetSymbol === 'USDT' || e.assetSymbol === 'USDC';
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
				if (selectedNode?.id) {
					cy.nodes().unselect();
					cy.getElementById(selectedNode.id).select();
				}
				return;
			}

			if (newElements.length > 0 && cy.elements().length > 0) {
				cy.batch(() => {
					cy.add(newElements);
				});
				if (selectedNode?.id) {
					cy.nodes().unselect();
					cy.getElementById(selectedNode.id).select();
				}
				const layout = cy.layout({
					name: effectiveLayout,
					fit: false,
					animate: true,
				});
				layout.run();
				return;
			}

			cy.batch(() => {
				cy.elements().remove();
				cy.add(elements);
			});
			if (selectedNode?.id) {
				cy.nodes().unselect();
				cy.getElementById(selectedNode.id).select();
			}
			const layout = cy.layout({
				name: effectiveLayout,
				fit: true,
				padding: 80,
				animate: true,
			});
			layout.run();
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
						'border-width': (ele: cytoscape.NodeSingular) => (ele.data('is_seed') ? 3 : 2),
						'border-color': 'data(borderColor)',
						'border-opacity': 1,
						'overlay-padding': '4px',
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
				{
					selector: 'node:selected',
					style: {
						'border-width': 5,
						'border-color': '#34D399',
						'border-opacity': 1,
						'line-color': '#34D399',
						'target-arrow-color': '#34D399',
						opacity: 1,
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

		cy.minZoom(0.3);
		cy.maxZoom(2.0);

		cy.on('tap', 'node', (evt) => {
			const nData = evt.target.data('raw');
			setSelectedNode(nData);
			setSelectedEdge(null);
			onNodeSelectRef.current(nData);
		});
		cy.on('tap', 'edge', (evt) => {
			setSelectedEdge(evt.target.data('raw'));
		});
		cy.on('tap', (evt) => {
			if (evt.target === cy) {
				setSelectedNode(null);
				setSelectedEdge(null);
				onNodeSelectRef.current(null);
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
	const activeTargetAddress = selectedNode?.id || seedNode?.id || graphData?.seed_address || '';

	const toolBtn =
		'p-1.5 rounded-lg transition hover:bg-[var(--slate)] text-[var(--ink-3)] hover:text-[var(--ink)]';

	return (
		<div className="relative w-full h-full flex flex-col" style={{ background: 'var(--snow)' }}>
			{/* Index Sync Warning Banner (Lucide AlertTriangle icon - NO emojis) */}
			{graphData?.sync_state?.warning_message && (
				<div
					className="px-4 py-2 flex items-center gap-2 text-xs shrink-0"
					style={{
						background: '#FEF3C7',
						borderBottom: '1px solid #FCD34D',
						color: '#92400E',
					}}
				>
					<AlertTriangle className="w-4 h-4 text-amber-600 shrink-0" />
					<span className="font-medium">{graphData.sync_state.warning_message}</span>
				</div>
			)}

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
							disabled={!activeTargetAddress}
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
							Expand
						</button>
						<button
							type="button"
							disabled={!activeTargetAddress}
							onClick={() => activeTargetAddress && onCollapseNode?.(activeTargetAddress)}
							className="btn-outline text-[11px] flex items-center gap-1 transition font-medium"
							style={{ padding: '0.25rem 0.625rem' }}
							title={
								selectedNode ? `Collapse expanded branch for ${selectedNode.id}` : 'Collapse branch'
							}
						>
							<MinusCircle className="w-3.5 h-3.5 text-slate-500" />
							Collapse
						</button>
					</div>
				</div>

				{/* Right: layout + actions */}

				<div className="flex items-center gap-2">
					{/* Undo / Redo history controls */}
					<div className="flex items-center gap-1 mr-1">
						<button
							type="button"
							onClick={onUndo}
							disabled={!canUndo}
							className={`p-1.5 rounded-lg transition ${
								canUndo
									? 'hover:bg-[var(--slate)] text-[var(--ink-2)]'
									: 'text-[var(--ink-3)] opacity-40 cursor-not-allowed'
							}`}
							title="Undo Graph Change (Ctrl+Z)"
						>
							<Undo2 className="w-3.5 h-3.5" />
						</button>
						<button
							type="button"
							onClick={onRedo}
							disabled={!canRedo}
							className={`p-1.5 rounded-lg transition ${
								canRedo
									? 'hover:bg-[var(--slate)] text-[var(--ink-2)]'
									: 'text-[var(--ink-3)] opacity-40 cursor-not-allowed'
							}`}
							title="Redo Graph Change (Ctrl+Y)"
						>
							<Redo2 className="w-3.5 h-3.5" />
						</button>
					</div>

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

					<button
						type="button"
						onClick={onShareCanvas}
						className="btn-primary text-[11px] flex items-center gap-1.5"
						style={{ padding: '0.3rem 0.75rem' }}
					>
						<Share2 className="w-3.5 h-3.5" />
						Share
					</button>

					{onExportCase && (
						<button
							type="button"
							onClick={onExportCase}
							className="btn-outline text-[11px] flex items-center gap-1.5"
							style={{ padding: '0.3rem 0.75rem' }}
						>
							<Download className="w-3.5 h-3.5" />
							Export
						</button>
					)}
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
