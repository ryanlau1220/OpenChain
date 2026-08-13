import { Link2, Pin } from 'lucide-react';
import type React from 'react';
import {
	type GraphEdge,
	type GraphNode,
	type SupportedNetwork,
	explorerURL,
} from '../services/api';

export type EvidencePath = {
	label: string;
	transferId: string;
	transactionHash: string;
	timestamp: bigint;
	asset: string;
	amount: string;
	provenance: string;
};

const evidenceTime = (timestamp: bigint) => {
	const date = new Date(Number(timestamp) * 1000);
	return Number.isFinite(date.getTime()) ? date.toISOString() : 'Unknown observation time';
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
			return [
				{
					label: label.label,
					transferId: edge.id,
					transactionHash: edge.transactionHash,
					timestamp: edge.lastTxTimestamp,
					asset: edge.asset?.symbol || 'Unknown asset',
					amount: edge.amountFormatted || `${edge.amountBaseUnits || 'Unknown amount'} raw units`,
					provenance: `${edge.sourceName || 'Unknown provider'} · retrieved ${evidenceTime(edge.retrievedAt)}`,
				},
			];
		});
	});
}

export function orderedEvidencePaths(
	paths: readonly EvidencePath[],
	pinnedTransferIds: readonly string[],
): EvidencePath[] {
	const pinned = new Set(pinnedTransferIds);
	return paths.toSorted(
		(left, right) => Number(pinned.has(right.transferId)) - Number(pinned.has(left.transferId)),
	);
}

export const EvidencePaths: React.FC<{
	nodes: readonly GraphNode[];
	edges: readonly GraphEdge[];
	network: SupportedNetwork;
	pinnedTransferIds: readonly string[];
	onTogglePin: (transferId: string) => void;
}> = ({ nodes, edges, network, pinnedTransferIds, onTogglePin }) => {
	const paths = orderedEvidencePaths(evidencePaths(nodes, edges), pinnedTransferIds);
	const pinned = new Set(pinnedTransferIds);
	const visiblePaths = [
		...paths.filter((path) => pinned.has(path.transferId)),
		...paths.filter((path) => !pinned.has(path.transferId)).slice(0, 5),
	];
	if (visiblePaths.length === 0) return null;
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
				Direct labels only; no risk inference. Pinned paths remain while expanding and export with
				the case.
			</p>
			{visiblePaths.map((path) => (
				<div
					key={`${path.label}:${path.transferId}`}
					className="rounded-lg p-2 text-[10px]"
					style={{ background: 'var(--white)', border: '1px solid var(--border)' }}
				>
					<div className="flex items-start justify-between gap-2">
						<p className="font-semibold" style={{ color: 'var(--ink)' }}>
							{path.label}
						</p>
						<button
							type="button"
							onClick={() => onTogglePin(path.transferId)}
							aria-pressed={pinned.has(path.transferId)}
							aria-label={`${pinned.has(path.transferId) ? 'Unpin' : 'Pin'} evidence ${path.transferId}`}
							className="shrink-0 rounded p-1"
							style={{ color: pinned.has(path.transferId) ? '#059669' : 'var(--ink-3)' }}
						>
							<Pin
								className="h-3.5 w-3.5"
								fill={pinned.has(path.transferId) ? 'currentColor' : 'none'}
							/>
						</button>
					</div>
					<a
						href={explorerURL(network, 'tx', path.transactionHash)}
						target="_blank"
						rel="noreferrer"
						className="font-mono break-all hover:underline"
						style={{ color: 'var(--accent)' }}
						title={path.transferId}
					>
						{path.transactionHash}
					</a>
					<p className="mt-1 font-mono text-[9px]" style={{ color: 'var(--ink-2)' }}>
						{path.asset} · {path.amount}
					</p>
					<p className="mt-1 text-[9px]" style={{ color: 'var(--ink-3)' }}>
						Observed {evidenceTime(path.timestamp)}
					</p>
					<p className="mt-1 break-all text-[9px]" style={{ color: 'var(--ink-3)' }}>
						Provenance: {path.provenance}
					</p>
				</div>
			))}
		</section>
	);
};
