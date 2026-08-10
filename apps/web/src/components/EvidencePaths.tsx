import { Link2 } from 'lucide-react';
import type React from 'react';
import { type EVMNetwork, type GraphEdge, type GraphNode, explorerURL } from '../services/api';

export type EvidencePath = {
	label: string;
	transferId: string;
	transactionHash: string;
};

export function evidencePaths(
	nodes: readonly GraphNode[],
	edges: readonly GraphEdge[],
): EvidencePath[] {
	const labelsByAddress = new Map(nodes.map((node) => [node.id, node.labels]));
	const seen = new Set<string>();
	return edges.flatMap((edge) => {
		const labels = [
			...(labelsByAddress.get(edge.source) ?? []),
			...(labelsByAddress.get(edge.target) ?? []),
		];
		return labels.flatMap((label) => {
			const key = `${label.id}:${edge.id}`;
			if (seen.has(key)) return [];
			seen.add(key);
			return [{ label: label.label, transferId: edge.id, transactionHash: edge.transactionHash }];
		});
	});
}

export const EvidencePaths: React.FC<{
	nodes: readonly GraphNode[];
	edges: readonly GraphEdge[];
	network: EVMNetwork;
}> = ({ nodes, edges, network }) => {
	const paths = evidencePaths(nodes, edges).slice(0, 5);
	if (paths.length === 0) return null;
	return (
		<section className="space-y-2 border-t pt-4" style={{ borderColor: 'var(--border)' }}>
			<div className="flex items-center gap-1.5">
				<Link2 className="w-3.5 h-3.5" style={{ color: 'var(--accent)' }} />
				<h3
					className="text-[10px] uppercase font-bold tracking-widest"
					style={{ color: 'var(--ink-3)' }}
				>
					Evidence paths
				</h3>
			</div>
			<p className="text-[10px]" style={{ color: 'var(--ink-3)' }}>
				Direct labels only; no risk inference.
			</p>
			{paths.map((path) => (
				<div
					key={`${path.label}:${path.transferId}`}
					className="rounded-lg p-2 text-[10px]"
					style={{ background: 'var(--white)', border: '1px solid var(--border)' }}
				>
					<p className="font-semibold" style={{ color: 'var(--ink)' }}>
						{path.label}
					</p>
					<a
						href={explorerURL(network, 'tx', path.transactionHash)}
						target="_blank"
						rel="noreferrer"
						className="font-mono break-all hover:underline"
						style={{ color: 'var(--accent)' }}
						title={path.transferId}
					>
						{path.transferId}
					</a>
				</div>
			))}
		</section>
	);
};
