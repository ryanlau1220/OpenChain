import { type EVMNetwork, type GraphEdge, explorerURL } from '../services/api';

export function TransferInspector({
	edge,
	network,
}: { edge: GraphEdge | null; network: EVMNetwork }) {
	if (!edge) return null;
	const asset = edge.asset;
	const rows = [
		['Kind', edge.transferKind],
		['Amount', edge.amountFormatted],
		['Raw units', edge.amountBaseUnits],
		['Asset', asset?.symbol || 'Unknown'],
		['Asset type', asset?.kind || 'Unknown'],
		['Contract', asset?.contractAddress || 'Native ETH'],
		['Decimals', String(asset?.decimals ?? 0)],
		['Event ID', edge.eventId],
		['Block', String(edge.blockNumber)],
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
				Transfer evidence
			</h3>
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
