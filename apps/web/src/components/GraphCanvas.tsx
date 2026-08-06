import cytoscape, { type Core } from 'cytoscape';
import {
	ExternalLink,
	Layers,
	Maximize2,
	PlusCircle,
	ZoomIn,
	ZoomOut,
} from 'lucide-react';
import type React from 'react';
import { useEffect, useRef, useState } from 'react';
import type { GraphData, GraphEdge, GraphNode } from '../services/api';

interface GraphCanvasProps {
	graphData: GraphData | null;
	onNodeSelect: (node: GraphNode) => void;
	onExpandNode: (address: string) => void;
}

export const GraphCanvas: React.FC<GraphCanvasProps> = ({
	graphData,
	onNodeSelect,
	onExpandNode,
}) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const cyRef = useRef<Core | null>(null);
	const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
	const [_selectedEdge, setSelectedEdge] = useState<GraphEdge | null>(null);
	const [layoutName, setLayoutName] = useState<
		'cose' | 'concentric' | 'breadthfirst' | 'grid'
	>('cose');
	const [_hopDepth, _setHopDepth] = useState<number>(2);

	useEffect(() => {
		if (!containerRef.current || !graphData) return;

		const elements: cytoscape.ElementDefinition[] = [];

		// Map Nodes
		graphData.nodes.forEach((n) => {
			let bg = '#3B82F6'; // Default Blue
			if (n.is_seed)
				bg = '#06B6D4'; // Seed Cyan
			else if (n.risk_score >= 50)
				bg = '#EF4444'; // Red Risk
			else if (n.entity_type === 'CONTRACT') bg = '#8B5CF6'; // Purple Contract

			elements.push({
				group: 'nodes',
				data: {
					id: n.id,
					label: n.label || n.id.substring(0, 8),
					risk_score: n.risk_score,
					is_seed: n.is_seed,
					entity_type: n.entity_type,
					bg: bg,
					raw: n,
				},
			});
		});

		// Map Edges
		graphData.edges.forEach((e) => {
			elements.push({
				group: 'edges',
				data: {
					id: e.id,
					source: e.source,
					target: e.target,
					label: e.value_formatted || 'Transfer',
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
						'font-size': '11px',
						'font-family': 'Inter, sans-serif',
						'text-valign': 'bottom',
						'text-margin-y': 6,
						width: (ele: cytoscape.NodeSingular) =>
							ele.data('is_seed') ? 48 : 36,
						height: (ele: cytoscape.NodeSingular) =>
							ele.data('is_seed') ? 48 : 36,
						'border-width': (ele: cytoscape.NodeSingular) =>
							ele.data('is_seed') ? 3 : 1,
						'border-color': '#FFFFFF',
						'overlay-padding': '4px',
					},
				},
				{
					selector: 'edge',
					style: {
						width: 2,
						'line-color': '#334155',
						'target-arrow-color': '#64748B',
						'target-arrow-shape': 'triangle',
						'curve-style': 'bezier',
						label: 'data(label)',
						'font-size': '9px',
						color: '#94A3B8',
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
						'border-color': '#06B6D4',
						'line-color': '#06B6D4',
						'target-arrow-color': '#06B6D4',
					},
				},
			],
			layout: {
				name: layoutName,
				padding: 50,
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
			setSelectedNode(null);
		});

		cyRef.current = cy;

		return () => {
			cy.destroy();
		};
	}, [graphData, layoutName]);

	const handleZoomIn = () => cyRef.current?.zoom(cyRef.current.zoom() * 1.25);
	const handleZoomOut = () => cyRef.current?.zoom(cyRef.current.zoom() * 0.8);
	const handleFit = () => cyRef.current?.fit();

	return (
		<div className="relative w-full h-[calc(100vh-7rem)] bg-slate-950 flex flex-col overflow-hidden">
			{/* Top Toolbar */}
			<div className="absolute top-4 left-4 z-10 flex items-center gap-2 p-2 glass-panel rounded-xl shadow-2xl">
				<button
					onClick={handleZoomIn}
					className="p-2 rounded-lg hover:bg-slate-800 text-slate-300 transition"
					title="Zoom In"
				>
					<ZoomIn className="w-4 h-4" />
				</button>
				<button
					onClick={handleZoomOut}
					className="p-2 rounded-lg hover:bg-slate-800 text-slate-300 transition"
					title="Zoom Out"
				>
					<ZoomOut className="w-4 h-4" />
				</button>
				<button
					onClick={handleFit}
					className="p-2 rounded-lg hover:bg-slate-800 text-slate-300 transition"
					title="Fit Canvas"
				>
					<Maximize2 className="w-4 h-4" />
				</button>
				<div className="h-4 w-[1px] bg-slate-800 my-auto mx-1" />
				<div className="flex items-center gap-2 text-xs text-slate-300 px-2">
					<Layers className="w-3.5 h-3.5 text-cyan-400" />
					<span>Layout:</span>
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
						className="bg-slate-900 border border-slate-700 text-slate-200 text-xs rounded-md px-2 py-1 focus:outline-none"
					>
						<option value="cose">Force (Cose)</option>
						<option value="concentric">Concentric</option>
						<option value="breadthfirst">Hierarchical</option>
						<option value="grid">Grid</option>
					</select>
				</div>
			</div>

			{/* Graph Statistics Header */}
			{graphData && (
				<div className="absolute top-4 right-4 z-10 flex items-center gap-4 px-4 py-2 glass-panel rounded-xl text-xs text-slate-300">
					<div>
						Seed:{' '}
						<span className="font-mono font-semibold text-cyan-400">
							{graphData.seed_address.substring(0, 10)}...
						</span>
					</div>
					<div className="h-3 w-[1px] bg-slate-800" />
					<div>
						Nodes:{' '}
						<span className="font-bold text-white">
							{graphData.total_nodes}
						</span>
					</div>
					<div>
						Edges:{' '}
						<span className="font-bold text-white">
							{graphData.total_edges}
						</span>
					</div>
				</div>
			)}

			{/* Cytoscape Canvas Render Container */}
			<div
				ref={containerRef}
				className="w-full h-full cursor-grab active:cursor-grabbing"
			/>

			{/* Selected Element Detail Drawer */}
			{selectedNode && (
				<div className="absolute bottom-4 left-4 z-10 w-96 p-4 glass-panel rounded-2xl shadow-2xl border border-cyan-500/30 animate-in fade-in slide-in-from-bottom-4">
					<div className="flex items-center justify-between mb-3">
						<div className="flex items-center gap-2">
							<div className="w-2.5 h-2.5 rounded-full bg-cyan-400 animate-pulse" />
							<span className="text-xs font-bold uppercase tracking-wider text-slate-400">
								Node Properties
							</span>
						</div>
						<button
							onClick={() => setSelectedNode(null)}
							className="text-xs text-slate-400 hover:text-white"
						>
							✕
						</button>
					</div>

					<div className="space-y-2 text-xs">
						<div>
							<span className="text-slate-400">Address:</span>
							<p className="font-mono text-cyan-300 break-all bg-slate-900/80 p-2 rounded-lg mt-0.5 border border-slate-800">
								{selectedNode.id}
							</p>
						</div>
						<div className="flex justify-between items-center py-1">
							<span className="text-slate-400">Entity Type:</span>
							<span className="font-semibold text-slate-200">
								{selectedNode.entity_type}
							</span>
						</div>
						<div className="flex justify-between items-center py-1">
							<span className="text-slate-400">Risk Score:</span>
							<span
								className={`font-bold ${selectedNode.risk_score >= 50 ? 'text-red-400' : 'text-emerald-400'}`}
							>
								{selectedNode.risk_score} / 100
							</span>
						</div>

						<div className="pt-3 flex gap-2">
							<button
								onClick={() => onExpandNode(selectedNode.id)}
								className="flex-1 flex items-center justify-center gap-1.5 py-2 px-3 bg-cyan-500 hover:bg-cyan-400 text-slate-950 font-bold rounded-xl transition text-xs shadow-md shadow-cyan-500/20"
							>
								<PlusCircle className="w-3.5 h-3.5" />
								Expand Counterparties
							</button>
							<a
								href={`https://sepolia.etherscan.io/address/${selectedNode.id}`}
								target="_blank"
								rel="noreferrer"
								className="p-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl transition flex items-center justify-center"
								title="View on Etherscan"
							>
								<ExternalLink className="w-3.5 h-3.5" />
							</a>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};
