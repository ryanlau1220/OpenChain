import cytoscape from 'cytoscape';
import {
	ArrowLeftRight,
	ArrowRight,
	Download,
	Layers,
	Maximize2,
	RotateCcw,
	Share2,
	ZoomIn,
	ZoomOut,
} from 'lucide-react';
import type React from 'react';
import { useEffect, useRef, useState } from 'react';
import type { GraphData, GraphEdge, GraphNode } from '../services/api';

interface GraphCanvasProps {
	graphData: GraphData | null;
	onNodeSelect: (node: GraphNode | null) => void;
	onExpandNode?: (
		address: string,
		direction: 'INFLOW' | 'OUTFLOW' | 'BOTH',
	) => void;
	onShareCanvas?: () => void;
	onExportCase?: () => void;
}

export const GraphCanvas: React.FC<GraphCanvasProps> = ({
	graphData,
	onNodeSelect,
	onExpandNode,
	onShareCanvas,
	onExportCase,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const cyRef = useRef<cytoscape.Core | null>(null);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
	const [_selectedEdge, setSelectedEdge] = useState<GraphEdge | null>(null);
	const [layoutName, setLayoutName] = useState<
		'cose' | 'concentric' | 'breadthfirst' | 'grid'
	>('breadthfirst');

	useEffect(() => {
		if (!containerRef.current || !graphData) return;

		const elements: cytoscape.ElementDefinition[] = [];

		// Map Nodes
		(graphData?.nodes || []).forEach((n) => {
			let bg = '#2563EB'; // Default Blue EOA
			let badge = n.entity_type || 'EOA';

			if (n.is_seed) {
				bg = '#0284C7'; // Target Seed Cyan/Sky
				badge = 'Target Wallet';
			} else if (n.entity_type === 'SCAMMER' || n.risk_score >= 50) {
				bg = '#DC2626'; // Red Risk / Scammer
				badge = 'Scammer';
			} else if (n.entity_type === 'EXCHANGE') {
				bg = '#D97706'; // Amber Exchange
				badge = 'Exchange';
			} else if (n.entity_type === 'CONTRACT') {
				bg = '#7C3AED'; // Purple Contract
				badge = 'Contract';
			}

			const inCount = n.in_tx_count ?? 0;
			const outCount = n.out_tx_count ?? 0;
			const countStr = `${outCount} Out / ${inCount} In`;
			const displayLabel = `${countStr}\n${n.label || n.id.substring(0, 8)}`;

			elements.push({
				group: 'nodes',
				data: {
					id: n.id,
					label: displayLabel,
					badge: badge,
					risk_score: n.risk_score,
					is_seed: n.is_seed,
					entity_type: n.entity_type,
					bg: bg,
					raw: n,
				},
			});
		});

		// Map Edges
		(graphData?.edges || []).forEach((e) => {
			const isToken = e.asset_symbol === 'USDT' || e.asset_symbol === 'USDC';
			const edgeColor = isToken ? '#059669' : '#DC2626'; // Green for Token, Red for ETH

			elements.push({
				group: 'edges',
				data: {
					id: e.id,
					source: e.source,
					target: e.target,
					label: `${e.tx_count} Tx / Total ${e.value_formatted || 'Transfer'}`,
					color: edgeColor,
					raw: e,
				},
			});
		});

		const cy = cytoscape({
			container: containerRef.current,
			elements: elements,
			style: [
				{
					selector: 'node',
					style: {
						'background-color': 'data(bg)',
						label: 'data(label)',
						color: '#F8FAFC',
						'font-size': '10px',
						'font-family': 'Inter, sans-serif',
						'text-valign': 'top',
						'text-margin-y': -8,
						'text-wrap': 'wrap',
						'text-max-width': '120px',
						width: (ele: cytoscape.NodeSingular) =>
							ele.data('is_seed') ? 50 : 36,
						height: (ele: cytoscape.NodeSingular) =>
							ele.data('is_seed') ? 50 : 36,
						'border-width': (ele: cytoscape.NodeSingular) =>
							ele.data('is_seed') ? 3 : 1.5,
						'border-color': (ele: cytoscape.NodeSingular) =>
							ele.data('is_seed') ? '#38BDF8' : '#FFFFFF',
						'overlay-padding': '4px',
					},
				},
				{
					selector: 'edge',
					style: {
						width: 2,
						'line-color': 'data(color)',
						'target-arrow-color': 'data(color)',
						'target-arrow-shape': 'triangle',
						'curve-style': 'bezier',
						label: 'data(label)',
						'font-size': '9px',
						color: '#E2E8F0',
						'text-background-opacity': 1,
						'text-background-color': '#0F172A',
						'text-background-padding': '3px',
						'text-background-shape': 'roundrectangle',
					},
				},
				{
					selector: ':selected',
					style: {
						'border-width': 4,
						'border-color': '#38BDF8',
						'line-color': '#38BDF8',
						'target-arrow-color': '#38BDF8',
					},
				},
			],
			layout: {
				name: layoutName === 'breadthfirst' ? 'breadthfirst' : layoutName,
				directed: true,
				padding: 60,
				animate: true,
			},
		});

		cy.on('tap', 'node', (evt) => {
			const nData = evt.target.data('raw');
			setSelectedNode(nData);
			setSelectedEdge(null);
			onNodeSelect(nData);
		});

		cy.on('tap', 'edge', (evt) => {
			const eData = evt.target.data('raw');
			setSelectedEdge(eData);
		});

		cy.on('tap', (evt) => {
			if (evt.target === cy) {
				setSelectedNode(null);
				setSelectedEdge(null);
				onNodeSelect(null);
			}
		});

		cyRef.current = cy;

		return () => {
			cy.destroy();
		};
	}, [graphData, layoutName, onNodeSelect]);

	const handleZoomIn = () => cyRef.current?.zoom(cyRef.current.zoom() * 1.2);
	const handleZoomOut = () => cyRef.current?.zoom(cyRef.current.zoom() * 0.8);
	const handleFit = () => cyRef.current?.fit();
	const handleReset = () => {
		cyRef.current?.reset();
		cyRef.current?.fit();
	};

	return (
		<div className="relative w-full h-full bg-slate-950 flex flex-col">
			{/* Beosin Investigation Action Toolbar */}
			<div className="h-12 border-b border-slate-800 bg-slate-900 px-4 flex items-center justify-between text-xs sticky top-0 z-20">
				<div className="flex items-center gap-3">
					<div className="flex items-center gap-1.5 text-slate-300 font-medium">
						<Layers className="w-3.5 h-3.5 text-blue-400" />
						<span>Selected:</span>
						<span className="text-blue-400 font-semibold">
							{selectedNode ? '1 address' : '0 addresses'}
						</span>
					</div>

					{/* Directional Hop Expansion Actions */}
					{selectedNode && (
						<div className="flex items-center gap-2 ml-3 pl-3 border-l border-slate-800">
							<button
								type="button"
								onClick={() => onExpandNode?.(selectedNode.id, 'INFLOW')}
								className="px-2.5 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 rounded font-medium transition flex items-center gap-1"
							>
								<ArrowRight className="w-3 h-3 rotate-180 text-blue-400" />
								Expand Inflow
							</button>
							<button
								type="button"
								onClick={() => onExpandNode?.(selectedNode.id, 'OUTFLOW')}
								className="px-2.5 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 rounded font-medium transition flex items-center gap-1"
							>
								<ArrowRight className="w-3 h-3 text-blue-400" />
								Expand Outflow
							</button>
						</div>
					)}
				</div>

				{/* Layout Selector & Canvas Actions */}
				<div className="flex items-center gap-2">
					<select
						value={layoutName}
						onChange={(e) =>
							setLayoutName(
								e.target.value as
									| 'cose'
									| 'concentric'
									| 'breadthfirst'
									| 'grid',
							)
						}
						className="bg-slate-950 border border-slate-700 text-slate-200 text-xs rounded px-2.5 py-1 focus:outline-none"
					>
						<option value="breadthfirst">Flow Layout (Left to Right)</option>
						<option value="cose">Force Directed (Cose)</option>
						<option value="concentric">Concentric Circles</option>
						<option value="grid">Grid Layout</option>
					</select>

					<button
						type="button"
						onClick={onShareCanvas}
						className="px-3 py-1 bg-blue-600 hover:bg-blue-500 text-white font-medium rounded flex items-center gap-1.5 transition shadow"
					>
						<Share2 className="w-3.5 h-3.5" />
						Share Canvas
					</button>

					{onExportCase && (
						<button
							type="button"
							onClick={onExportCase}
							className="px-3 py-1 bg-slate-800 hover:bg-slate-700 text-slate-300 font-medium rounded flex items-center gap-1.5 transition"
						>
							<Download className="w-3.5 h-3.5" />
							Export Report
						</button>
					)}
				</div>
			</div>

			{/* Cytoscape Canvas */}
			<div
				ref={containerRef}
				className="flex-1 w-full h-full cursor-grab active:cursor-grabbing"
			/>

			{/* Floating Zoom & Fit Controls */}
			<div className="absolute bottom-6 left-6 flex items-center gap-1 p-1 bg-slate-900 border border-slate-800 rounded-lg shadow-xl z-10">
				<button
					type="button"
					onClick={handleZoomIn}
					className="p-1.5 hover:bg-slate-800 rounded text-slate-400 hover:text-white transition"
					title="Zoom In"
				>
					<ZoomIn className="w-4 h-4" />
				</button>
				<button
					type="button"
					onClick={handleZoomOut}
					className="p-1.5 hover:bg-slate-800 rounded text-slate-400 hover:text-white transition"
					title="Zoom Out"
				>
					<ZoomOut className="w-4 h-4" />
				</button>
				<button
					type="button"
					onClick={handleFit}
					className="p-1.5 hover:bg-slate-800 rounded text-slate-400 hover:text-white transition"
					title="Fit View"
				>
					<Maximize2 className="w-4 h-4" />
				</button>
				<button
					type="button"
					onClick={handleReset}
					className="p-1.5 hover:bg-slate-800 rounded text-slate-400 hover:text-white transition"
					title="Reset View"
				>
					<RotateCcw className="w-4 h-4" />
				</button>
			</div>

			{/* Empty State Overlay */}
			{(!graphData || (graphData.nodes || []).length === 0) && (
				<div className="absolute inset-0 flex flex-col items-center justify-center bg-slate-950/90 pointer-events-none">
					<ArrowLeftRight className="w-10 h-10 text-slate-600 mb-2" />
					<h3 className="text-sm font-semibold text-slate-300">
						No Address Flow Rendered
					</h3>
					<p className="text-xs text-slate-500 mt-1">
						Enter target address(es) above to start tracing
					</p>
				</div>
			)}
		</div>
	);
};
