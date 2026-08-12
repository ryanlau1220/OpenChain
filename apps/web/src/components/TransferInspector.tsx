import { type GraphEdge, type SupportedNetwork, explorerURL } from '../services/api';

export function TransferInspector({
	edge,
	edges = [],
	network,
	onSelect,
}: {
	edge: GraphEdge | null;
	edges?: readonly GraphEdge[];
	network: SupportedNetwork;
	onSelect?: (edge: GraphEdge) => void;
}) {
	const transfers = edges.length > 0 ? edges : edge ? [edge] : [];
	const inspected = transfers.find((item) => item.id === edge?.id) ?? transfers[0];
	if (!inspected) return null;
	edge = inspected;
	const asset = edge.asset;
	const rows = [
		['Kind', edge.transferKind],
		['Amount', edge.amountFormatted],
		['Raw units', edge.amountBaseUnits],
		['Asset', asset?.symbol || 'Unknown'],
		['Asset type', asset?.kind || 'Unknown'],
		['Contract', asset?.contractAddress || 'Native asset'],
		['Decimals', String(asset?.decimals ?? 0)],
		['Event ID', edge.eventId],
		['Block', String(edge.blockNumber)],
		['Finality', edge.provisional ? 'Provisional observation' : 'Finalized observation'],
		['Source', edge.sourceName],
		[
			'Retrieved',
			edge.retrievedAt ? new Date(Number(edge.retrievedAt) * 1000).toISOString() : 'Unknown',
		],
	];
	return (
		<section className="space-y-2 border-t pt-4" style={{ borderColor: 'var(--border)' }}>
			<h3
				className="text-[10px] uppercase font-bold tracking-widest"
				style={{ color: 'var(--ink-3)' }}
			>
				{transfers.length > 1
					? `Relationship evidence · ${transfers.length} transfers`
					: 'Transfer evidence'}
			</h3>
			{transfers.length > 1 && (
				<div className="max-h-28 space-y-1 overflow-y-auto pr-1">
					{transfers.map((transfer) => (
						<button
							key={transfer.id}
							type="button"
							onClick={() => onSelect?.(transfer)}
							className="w-full break-all rounded px-2 py-1 text-left font-mono text-[10px]"
							style={{
								background: transfer.id === edge.id ? 'rgba(136,125,255,0.10)' : 'var(--slate)',
								color: transfer.id === edge.id ? 'var(--accent)' : 'var(--ink-2)',
							}}
						>
							{transfer.transactionHash} · {transfer.amountFormatted}
						</button>
					))}
				</div>
			)}
			<a
				href={explorerURL(network, 'tx', edge.transactionHash)}
				target="_blank"
				rel="noreferrer"
				className="font-mono break-all text-[10px] hover:underline"
				style={{ color: 'var(--accent)' }}
			>
				{edge.transactionHash}
			</a>
			<dl className="space-y-1 text-[10px]">
				{rows.map(([label, value]) => (
					<div key={label} className="grid grid-cols-[72px_1fr] gap-2">
						<dt style={{ color: 'var(--ink-3)' }}>{label}</dt>
						<dd className="font-mono break-all" style={{ color: 'var(--ink)' }}>
							{value}
						</dd>
					</div>
				))}
			</dl>
		</section>
	);
}
