import { Link2, Pin } from 'lucide-react';
import type React from 'react';
import { useState } from 'react';
import {
	type GraphEdge,
	type GraphNode,
	type SupportedNetwork,
	type TraceCoverage,
	explorerURL,
} from '../services/api';
import { formatObservationTime } from '../services/format';

export type EvidencePath = {
	label: string;
	transferId: string;
	transactionHash: string;
	timestamp: bigint;
	asset: string;
	amount: string;
	provenance: string;
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
					provenance: `${edge.sourceName || 'Unknown provider'} · retrieved ${formatObservationTime(edge.retrievedAt)}`,
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
	return [...paths].sort(
		(left, right) => Number(pinned.has(right.transferId)) - Number(pinned.has(left.transferId)),
	);
}

function formatCoverageTime(timestamp: bigint): string {
	return timestamp > 0n ? formatObservationTime(timestamp) : 'Unknown retrieval time';
}

export const EvidencePaths: React.FC<{
	nodes: readonly GraphNode[];
	edges: readonly GraphEdge[];
	network: SupportedNetwork;
	pinnedTransferIds: readonly string[];
	onTogglePin: (transferId: string) => void;
	coverage?: TraceCoverage;
	sourceName?: string;
}> = ({ nodes, edges, network, pinnedTransferIds, onTogglePin, coverage, sourceName }) => {
	const [showAll, setShowAll] = useState(false);
	const paths = orderedEvidencePaths(evidencePaths(nodes, edges), pinnedTransferIds);
	const pinned = new Set(pinnedTransferIds);
	const visiblePaths = showAll
		? paths
		: [
				...paths.filter((path) => pinned.has(path.transferId)),
				...paths.filter((path) => !pinned.has(path.transferId)).slice(0, 5),
			];
	if (visiblePaths.length === 0 && !coverage) return null;
	return (
		<section className="space-y-2 border-t pt-4" style={{ borderColor: 'var(--border)' }}>
			{coverage && coverage.requestedPageSize > 0 && (
				<div
					className="rounded-lg p-2 text-[9px]"
					style={{
						background: 'var(--slate)',
						border: '1px solid var(--border)',
						color: 'var(--ink-2)',
					}}
				>
					<p className="font-semibold uppercase tracking-wider" style={{ color: 'var(--ink-3)' }}>
						Fresh page & stored graph
					</p>
					<p className="mt-1">
						Fresh provider page: {sourceName || 'Unknown provider'} retrieved{' '}
						{formatCoverageTime(coverage.freshRetrievedAt)}. {coverage.observedTransferCount}{' '}
						observed; {coverage.ruleInputTransferCount} selected for deterministic rules (requested
						page size {coverage.requestedPageSize}).
					</p>
					<p className="mt-1">
						Fresh rule input: {coverage.confirmationBackedTransferCount} confirmation-backed ·{' '}
						{coverage.provisionalTransferCount} provisional.
					</p>
					<p className="mt-1">
						Stored AGE graph: {coverage.storedGraphTransferCount} edges ·{' '}
						{coverage.storedHistoryTransferCount} outside this fresh rule input. Stored edge
						retrieval range: {formatCoverageTime(coverage.storedOldestRetrievedAt)} –{' '}
						{formatCoverageTime(coverage.storedNewestRetrievedAt)}.
					</p>
					<p className="mt-1">
						{coverage.hasMore
							? 'Additional provider pages available'
							: coverage.providerComplete
								? 'Provider reports this page complete'
								: 'Provider completeness unavailable'}
						. {coverage.cursor ? 'Continuation provider page.' : 'First provider page.'}.
					</p>
					<p className="mt-1">{coverage.ruleInputScope}</p>
					<p className="mt-1">{coverage.limitation}</p>
				</div>
			)}
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
				Direct labels only; no risk inference. Each stored edge shows its original source and
				retrieval time, and may predate the fresh page above. Pinned paths remain while expanding
				and export with the case.
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
						Observed {formatObservationTime(path.timestamp)}
					</p>
					<p className="mt-1 break-all text-[9px]" style={{ color: 'var(--ink-3)' }}>
						Stored provenance: {path.provenance}
					</p>
				</div>
			))}
			{paths.length > visiblePaths.length && (
				<button
					type="button"
					onClick={() => setShowAll(true)}
					className="w-full rounded-lg border px-2 py-1.5 text-[10px] font-medium"
					style={{ borderColor: 'var(--border)', color: 'var(--accent)' }}
				>
					Show all {paths.length} evidence paths
				</button>
			)}
			{showAll && paths.length > 5 && (
				<button
					type="button"
					onClick={() => setShowAll(false)}
					className="w-full text-[10px]"
					style={{ color: 'var(--ink-3)' }}
				>
					Show fewer evidence paths
				</button>
			)}
		</section>
	);
};
