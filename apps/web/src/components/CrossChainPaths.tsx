import { ArrowRightLeft, ExternalLink, Pin, PinOff, Route } from 'lucide-react';
import type React from 'react';
import {
	BridgeLifecycle,
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
	pinnedTransitionIds?: readonly string[];
	onTogglePin?: (transitionID: string) => void;
	onTraceDestination?: (transition: CrossChainTransition) => void;
}> = ({ transitions, pinnedTransitionIds = [], onTogglePin, onTraceDestination }) => {
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
				Shown only with a canonical token route and bridge-specific event or message evidence. This
				does not establish cross-chain address ownership.
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
						<div className="flex items-start justify-between gap-2">
							<div>
								<p className="font-semibold" style={{ color: 'var(--ink)' }}>
									{transition.bridgeName}
								</p>
								<p
									className="mt-0.5 text-[9px] uppercase tracking-wide"
									style={{ color: lifecycleColor(transition.lifecycle) }}
								>
									{lifecycleLabel(transition.lifecycle)} ·{' '}
									{transition.sourceConfirmed ? 'source confirmed' : 'source pending'} ·{' '}
									{transition.destinationConfirmed
										? 'destination confirmed'
										: 'destination pending'}
								</p>
							</div>
							{onTogglePin && (
								<button
									type="button"
									title={
										pinnedTransitionIds.includes(transition.id)
											? 'Unpin bridge path'
											: 'Pin bridge path'
									}
									onClick={() => onTogglePin(transition.id)}
									className="rounded p-1 hover:bg-slate-100"
									style={{ color: 'var(--ink-3)' }}
								>
									{pinnedTransitionIds.includes(transition.id) ? (
										<Pin className="h-3.5 w-3.5" />
									) : (
										<PinOff className="h-3.5 w-3.5" />
									)}
								</button>
							)}
						</div>
						<p className="mt-1" style={{ color: 'var(--ink-2)' }}>
							{networkDetails(sourceNetwork).name} → {networkDetails(destinationNetwork).name}
							{' · '}
							{elapsed(transition.sourceTimestamp, transition.destinationTimestamp)}
						</p>
						<p className="mt-1 font-mono text-[9px]" style={{ color: 'var(--accent)' }}>
							{transition.asset?.symbol || transition.asset?.kind || 'Reported asset'} ·{' '}
							{transition.amountBaseUnits} raw units
						</p>
						<p className="mt-1 break-all font-mono text-[9px]" style={{ color: 'var(--ink-3)' }}>
							Message {transition.messageId || 'unavailable'} · route{' '}
							{transition.canonicalSourceToken || 'native'} →{' '}
							{transition.canonicalDestinationToken || 'native'}
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
								{transition.sourceBridgeAddress} ·{' '}
								{transition.sourceLogReference || 'log reference unavailable'}
							</p>
							{transition.destinationTransactionHash ? (
								<a
									href={explorerURL(
										destinationNetwork,
										'tx',
										transition.destinationTransactionHash,
									)}
									target="_blank"
									rel="noreferrer"
									className="flex items-center gap-1 break-all font-mono text-[9px] hover:underline"
									style={{ color: 'var(--accent)' }}
								>
									Destination transaction <ExternalLink className="h-3 w-3 shrink-0" />
								</a>
							) : (
								<p className="font-mono text-[9px]" style={{ color: 'var(--ink-3)' }}>
									Destination transaction pending
								</p>
							)}
							<p className="text-[9px]" style={{ color: 'var(--ink-3)' }}>
								{formatObservationTime(transition.destinationTimestamp)} · bridge{' '}
								{transition.destinationBridgeAddress} ·{' '}
								{transition.destinationLogReference || 'log reference unavailable'}
							</p>
						</div>
						<p className="mt-2 text-[9px]" style={{ color: 'var(--ink-3)' }}>
							Exact raw provenance: {transition.sourceAcquisitionIds.length} source /{' '}
							{transition.destinationAcquisitionIds.length} destination snapshots.
						</p>
						{onTraceDestination &&
							transition.recipient &&
							transition.lifecycle === BridgeLifecycle.FINALIZED && (
								<button
									type="button"
									onClick={() => onTraceDestination(transition)}
									className="mt-2 inline-flex items-center gap-1 rounded border px-2 py-1 text-[9px] font-semibold hover:bg-slate-50"
									style={{ borderColor: 'var(--border)', color: 'var(--ink)' }}
								>
									<Route className="h-3 w-3" /> Trace destination recipient
								</button>
							)}
						<p className="mt-2 text-[9px]" style={{ color: 'var(--ink-3)' }}>
							{transition.limitations}
						</p>
					</div>
				);
			})}
		</section>
	);
};

const lifecycleLabel = (value: BridgeLifecycle) => {
	switch (value) {
		case BridgeLifecycle.FINALIZED:
			return 'Finalized';
		case BridgeLifecycle.RELAYED:
			return 'Relayed; confirmation pending';
		case BridgeLifecycle.FAILED:
			return 'Relay attempt failed';
		case BridgeLifecycle.INITIATED:
			return 'Initiated; destination pending';
		default:
			return 'Unresolved evidence';
	}
};

const lifecycleColor = (value: BridgeLifecycle) =>
	value === BridgeLifecycle.FINALIZED
		? 'var(--success)'
		: value === BridgeLifecycle.FAILED
			? 'var(--danger)'
			: 'var(--ink-3)';
