import { Check, Clock, Copy, ExternalLink, Shield, Tag, Target, Wallet } from 'lucide-react';
import type React from 'react';
import { useState } from 'react';
import {
	type AddressLabel,
	type AddressSummary,
	LabelVisibility,
	LookupField,
	type LookupFieldStatus,
	type SupportedNetwork,
	entityLabel,
	explorerURL,
	networkDetails,
} from '../services/api';

interface WalletLookupProps {
	summary: AddressSummary | null;
	labels: AddressLabel[];
	fieldStatuses: LookupFieldStatus[];
	loading: boolean;
	onTraceAddress: (addr: string) => void;
	targetSeedAddress?: string;
	network: SupportedNetwork;
}

export function entityCategoryName(category: string): string {
	return (
		{
			exchange: 'Exchange',
			bridge: 'Bridge',
			merchant: 'Merchant',
			otc: 'OTC merchant',
			mixer: 'Mixer',
			'sanctioned-service': 'Sanctioned service',
			'high-risk-service': 'High-risk service',
			'defi-service': 'DeFi service',
			'burn-address': 'Burn address',
		}[category] || category
	);
}

const evidenceLink = (value: string) => {
	try {
		const url = new URL(value);
		return url.protocol === 'https:' ? url.href : '';
	} catch {
		return '';
	}
};

export const WalletLookup: React.FC<WalletLookupProps> = ({
	summary,
	labels,
	fieldStatuses,
	loading,
	onTraceAddress: _onTraceAddress,
	targetSeedAddress,
	network,
}) => {
	const [copied, setCopied] = useState<boolean>(false);

	const handleCopyAddress = async () => {
		if (!summary?.address) return;
		try {
			await navigator.clipboard.writeText(summary.address);
			setCopied(true);
			setTimeout(() => setCopied(false), 2000);
		} catch (err) {
			console.error('Failed to copy address:', err);
		}
	};

	if (loading) {
		return (
			<div className="p-6 flex flex-col items-center justify-center gap-3">
				{/* Prism spinner */}
				<div
					className="w-6 h-6 rounded-full border-2 border-transparent animate-spin"
					style={{
						borderTopColor: 'var(--accent)',
						borderRightColor: 'var(--prism-3)',
					}}
				/>
				<p className="text-xs" style={{ color: 'var(--ink-3)' }}>
					Querying blockchain data…
				</p>
			</div>
		);
	}

	if (!summary) {
		return (
			<div className="p-6 text-center text-xs" style={{ color: 'var(--ink-3)' }}>
				Select a node on the canvas to inspect wallet details and transaction history.
			</div>
		);
	}

	const safeLabels = Array.isArray(labels) ? labels : [];
	const lookupField = (field: LookupField) =>
		fieldStatuses.find((status) => status.field === field);
	const metricValue = (field: LookupField, value: string) =>
		lookupField(field)?.available ? value : 'Unavailable';

	const isSeedAddress =
		Boolean(summary.address) &&
		Boolean(targetSeedAddress) &&
		summary.address.toLowerCase() === targetSeedAddress?.toLowerCase();

	return (
		<div className="space-y-2 text-xs rise-in">
			{/* Address header */}
			<div
				className="p-3 rounded-xl space-y-2"
				style={{ background: 'var(--white)', border: '1px solid var(--border)' }}
			>
				<div className="flex items-center justify-between">
					<span
						className="whitespace-nowrap text-[8px] uppercase font-bold tracking-wider px-1.5 py-0.5 rounded"
						style={{
							color: isSeedAddress ? 'var(--accent)' : '#059669',
							background: isSeedAddress ? 'rgba(136, 125, 255, 0.12)' : 'rgba(52, 211, 153, 0.12)',
							border: isSeedAddress
								? '1px solid rgba(136, 125, 255, 0.3)'
								: '1px solid rgba(52, 211, 153, 0.3)',
						}}
					>
						{isSeedAddress ? 'Target Address' : 'Selected Address'}
					</span>
					<div className="flex items-center gap-1">
						<button
							type="button"
							onClick={handleCopyAddress}
							aria-label="Copy wallet address"
							className="flex h-6 w-6 items-center justify-center rounded transition"
							style={{
								background: copied ? 'rgba(52, 211, 153, 0.15)' : 'var(--slate)',
								color: copied ? '#059669' : 'var(--ink-2)',
								border: copied ? '1px solid rgba(52, 211, 153, 0.4)' : '1px solid var(--border)',
							}}
							title="Copy wallet address to clipboard"
						>
							{copied ? (
								<Check className="w-3 h-3 text-emerald-600" />
							) : (
								<Copy className="w-3 h-3 text-slate-500" />
							)}
							<output className="sr-only" aria-live="polite">
								{copied ? 'Copied' : ''}
							</output>
						</button>
						<a
							href={explorerURL(network, 'address', summary.address)}
							target="_blank"
							rel="noreferrer"
							aria-label="Open address in explorer"
							title="Open address in explorer"
							className="flex h-6 w-6 items-center justify-center rounded transition hover:text-[var(--accent)]"
							style={{ color: 'var(--ink-3)' }}
						>
							<ExternalLink className="w-3 h-3" />
						</a>
					</div>
				</div>

				<p
					className="font-mono font-semibold break-all select-all text-[11px]"
					style={{ color: 'var(--ink)' }}
				>
					{summary.address}
				</p>

				{!isSeedAddress && (
					<button
						type="button"
						onClick={() => _onTraceAddress(summary.address)}
						className="btn-primary w-full text-[11px] py-1.5 flex items-center justify-center gap-1.5 transition font-medium mt-2"
					>
						<Target className="w-3.5 h-3.5" />
						<span>Set as Main Target</span>
					</button>
				)}

				{/* Verified labels are evidence-backed registry data, never local case notes. */}
				{safeLabels.length > 0 && (
					<div className="space-y-1 pt-1">
						<span className="text-[9px] uppercase font-bold text-[var(--ink-3)] block">
							Verified entity labels
						</span>
						<p className="text-[9px]" style={{ color: 'var(--ink-3)' }}>
							Evidence-backed registry entries; separate from local annotations and findings.
						</p>
						<div className="flex flex-col gap-1">
							{safeLabels.map((l) => {
								const tier = l.trustTier ?? 1;
								const confidence = Math.round(Math.min(1, Math.max(0, l.confidence)) * 100);
								const proof = evidenceLink(l.evidenceUrl);
								return (
									<div
										key={l.id}
										className="p-1.5 rounded-lg flex items-center justify-between text-[10px]"
										style={{
											background:
												tier === 1
													? 'rgba(239,68,68,0.06)'
													: tier === 2
														? 'rgba(16,185,129,0.06)'
														: 'rgba(99,102,241,0.06)',
											border:
												tier === 1
													? '1px solid rgba(239,68,68,0.2)'
													: tier === 2
														? '1px solid rgba(16,185,129,0.2)'
														: '1px solid rgba(99,102,241,0.2)',
										}}
									>
										<div className="flex items-center gap-1.5">
											<Shield className="w-3.5 h-3.5 text-indigo-600 shrink-0" />
											<div>
												<span className="font-semibold block text-[var(--ink)]">
													{entityCategoryName(l.category)}: {l.label}
												</span>
												<span className="text-[9px] text-[var(--ink-3)]">
													{tier === 1
														? 'Tier 1 Authoritative'
														: tier === 2
															? 'Tier 2 Community Verified'
															: 'Tier 3 Workspace'}{' '}
													· {confidence}% confidence
												</span>
												<span className="text-[9px] text-[var(--ink-3)] block">
													Source: {l.source || 'Unavailable'} ·{' '}
													{l.sourceVersion || 'Version unavailable'}
												</span>
												<span className="text-[9px] text-[var(--ink-3)] block">
													{l.visibility === LabelVisibility.PUBLIC
														? 'Public registry entry'
														: 'Unspecified visibility'}
												</span>
											</div>
										</div>
										{proof && (
											<a
												href={proof}
												target="_blank"
												rel="noreferrer"
												className="text-indigo-600 hover:underline flex items-center gap-0.5 text-[9px]"
											>
												Proof <ExternalLink className="w-2.5 h-2.5" />
											</a>
										)}
									</div>
								);
							})}
						</div>
					</div>
				)}
			</div>

			{/* Metrics grid */}
			<div className="grid grid-cols-3 gap-1.5">
				{[
					{
						label: 'Balance',
						value: metricValue(LookupField.BALANCE, summary.balanceFormatted || '—'),
						warning: lookupField(LookupField.BALANCE)?.warning,
						icon: <Wallet className="w-3.5 h-3.5" style={{ color: 'var(--accent)' }} />,
					},
					...(networkDetails(network).activityLabel
						? [
								{
									label: networkDetails(network).activityLabel,
									value: metricValue(LookupField.ACTIVITY, String(summary.txCount ?? '—')),
									warning: lookupField(LookupField.ACTIVITY)?.warning,
									icon: <Clock className="w-3.5 h-3.5" style={{ color: 'var(--accent)' }} />,
								},
							]
						: []),
					{
						label: 'Entity',
						value: metricValue(LookupField.ENTITY_TYPE, entityLabel(summary.entityType)),
						warning: lookupField(LookupField.ENTITY_TYPE)?.warning,
						icon: <Tag className="w-3.5 h-3.5" style={{ color: 'var(--prism-4)' }} />,
					},
				].map(({ label, value, warning, icon }) => (
					<div
						key={label}
						className="min-w-0 p-2 rounded-xl space-y-1"
						style={{ background: 'var(--white)', border: '1px solid var(--border)' }}
					>
						<div
							className="flex items-center justify-between text-[10px]"
							style={{ color: 'var(--ink-3)' }}
						>
							<span className="truncate" title={warning || label}>
								{label}
							</span>
							{icon}
						</div>
						<p
							className="truncate font-semibold font-mono text-[11px]"
							style={{ color: 'var(--ink)' }}
						>
							{value}
						</p>
					</div>
				))}
			</div>
		</div>
	);
};
