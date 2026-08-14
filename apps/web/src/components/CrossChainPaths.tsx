import { ArrowRightLeft, ExternalLink } from 'lucide-react';
import type React from 'react';
import {
	type CrossChainTransition,
	type SupportedNetwork,
	explorerURL,
	networkDetails,
} from '../services/api';
import { formatObservationTime } from '../services/format';

const elapsed = (source: bigint, destination: bigint) => {
	const seconds = Number(destination - source);
	if (!Number.isFinite(seconds) || seconds < 0) return 'Unknown elapsed time';
	if (seconds < 60) return `${seconds}s later`;
	if (seconds < 3600) return `${Math.floor(seconds / 60)}m later`;
	return `${Math.floor(seconds / 3600)}h later`;
};

export const CrossChainPaths: React.FC<{
	transitions: readonly CrossChainTransition[];
}> = ({ transitions }) => {
	if (transitions.length === 0) return null;
	return (
		<section className="space-y-2 border-t pt-4" style={{ borderColor: 'var(--border)' }}>
			<div className="flex items-center gap-1.5">
				<ArrowRightLeft className="h-3.5 w-3.5" style={{ color: 'var(--accent)' }} />
				<h3
					className="text-[10px] uppercase font-bold tracking-widest"
					style={{ color: 'var(--ink-3)' }}
				>
					Cross-chain continuations
				</h3>
			</div>
			<p className="text-[10px]" style={{ color: 'var(--ink-3)' }}>
				Known bridge and matching transfer evidence only. This does not establish cross-chain
				address ownership.
			</p>
			{transitions.map((transition) => {
				const sourceNetwork = transition.sourceNetwork as SupportedNetwork;
				const destinationNetwork = transition.destinationNetwork as SupportedNetwork;
				return (
					<div
						key={transition.id}
						className="rounded-lg p-2 text-[10px]"
						style={{ background: 'var(--white)', border: '1px solid var(--border)' }}
					>
						<p className="font-semibold" style={{ color: 'var(--ink)' }}>
							{transition.bridgeName}
						</p>
						<p className="mt-1" style={{ color: 'var(--ink-2)' }}>
							{networkDetails(sourceNetwork).name} → {networkDetails(destinationNetwork).name}
							{' · '}
							{elapsed(transition.sourceTimestamp, transition.destinationTimestamp)}
						</p>
						<p className="mt-1 font-mono text-[9px]" style={{ color: 'var(--accent)' }}>
							{transition.asset?.symbol || transition.asset?.kind || 'Reported asset'} ·{' '}
							{transition.amountBaseUnits} raw units
						</p>
						<div className="mt-2 space-y-1">
							<a
								href={explorerURL(sourceNetwork, 'tx', transition.sourceTransactionHash)}
								target="_blank"
								rel="noreferrer"
								className="flex items-center gap-1 break-all font-mono text-[9px] hover:underline"
								style={{ color: 'var(--accent)' }}
							>
								Source transaction <ExternalLink className="h-3 w-3 shrink-0" />
							</a>
							<p className="text-[9px]" style={{ color: 'var(--ink-3)' }}>
								{formatObservationTime(transition.sourceTimestamp)} · bridge{' '}
								{transition.sourceBridgeAddress}
							</p>
							<a
								href={explorerURL(destinationNetwork, 'tx', transition.destinationTransactionHash)}
								target="_blank"
								rel="noreferrer"
								className="flex items-center gap-1 break-all font-mono text-[9px] hover:underline"
								style={{ color: 'var(--accent)' }}
							>
								Destination transaction <ExternalLink className="h-3 w-3 shrink-0" />
							</a>
							<p className="text-[9px]" style={{ color: 'var(--ink-3)' }}>
								{formatObservationTime(transition.destinationTimestamp)} · bridge{' '}
								{transition.destinationBridgeAddress}
							</p>
						</div>
						<p className="mt-2 text-[9px]" style={{ color: 'var(--ink-3)' }}>
							{transition.limitations}
						</p>
					</div>
				);
			})}
		</section>
	);
};
