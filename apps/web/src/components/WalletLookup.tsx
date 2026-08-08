import { Clock, ExternalLink, Shield, ShieldAlert, ShieldCheck, Tag, Wallet } from 'lucide-react';

import type React from 'react';
import {
	type AddressLabel,
	type AddressSummary,
	type RiskEvaluation,
	type RiskFlag,
	entityLabel,
} from '../services/api';

interface WalletLookupProps {
	summary: AddressSummary | null;
	risk: RiskEvaluation | null;
	labels: AddressLabel[];
	loading: boolean;
	onTraceAddress: (addr: string) => void;
}

export const WalletLookup: React.FC<WalletLookupProps> = ({
	summary,
	risk,
	labels,
	loading,
	onTraceAddress: _onTraceAddress,
}) => {
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
					Querying EVM RPC…
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
	const safeFlags = Array.isArray(risk?.flags) ? risk.flags : [];
	const riskScore = risk?.totalScore ?? 0;
	const isHighRisk = riskScore >= 50;

	return (
		<div className="space-y-3 text-xs rise-in">
			{/* Address header */}
			<div
				className="p-3 rounded-xl space-y-2"
				style={{ background: 'var(--white)', border: '1px solid var(--border)' }}
			>
				<div className="flex items-center justify-between">
					<span
						className="text-[9px] uppercase font-bold tracking-widest"
						style={{ color: 'var(--accent)' }}
					>
						Target Address
					</span>
					<a
						href={`https://sepolia.etherscan.io/address/${summary.address}`}
						target="_blank"
						rel="noreferrer"
						className="flex items-center gap-1 text-[10px] transition"
						style={{ color: 'var(--ink-3)' }}
					>
						Etherscan <ExternalLink className="w-3 h-3" />
					</a>
				</div>

				<p
					className="font-mono font-semibold break-all select-all text-[11px]"
					style={{ color: 'var(--ink)' }}
				>
					{summary.address}
				</p>

				{/* Labels as OLI Trust Tier pills */}
				{safeLabels.length > 0 && (
					<div className="space-y-1 pt-1">
						<span className="text-[9px] uppercase font-bold text-[var(--ink-3)] block">
							OLI Attested Labels
						</span>
						<div className="flex flex-col gap-1">
							{safeLabels.map((l) => {
								const tier = l.trustTier ?? 1;
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
											{tier === 1 ? (
												<ShieldAlert className="w-3.5 h-3.5 text-rose-600 shrink-0" />
											) : tier === 2 ? (
												<ShieldCheck className="w-3.5 h-3.5 text-emerald-600 shrink-0" />
											) : (
												<Shield className="w-3.5 h-3.5 text-indigo-600 shrink-0" />
											)}
											<div>
												<span className="font-semibold block text-[var(--ink)]">
													{l.category}: {l.label}
												</span>
												<span className="text-[9px] text-[var(--ink-3)]">
													{tier === 1
														? 'Tier 1 Authoritative'
														: tier === 2
															? 'Tier 2 Community Verified'
															: 'Tier 3 Workspace'}
												</span>
											</div>
										</div>
										{l.evidenceUrl && (
											<a
												href={l.evidenceUrl}
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
			<div className="grid grid-cols-2 gap-2">
				{[
					{
						label: 'Balance',
						value: summary.balanceFormatted || '—',
						icon: <Wallet className="w-3.5 h-3.5" style={{ color: 'var(--accent)' }} />,
					},
					{
						label: 'Tx Count',
						value: String(summary.txCount ?? '—'),
						icon: <Clock className="w-3.5 h-3.5" style={{ color: 'var(--accent)' }} />,
					},
					{
						label: 'Entity',
						value: entityLabel(summary.entityType),
						icon: <Tag className="w-3.5 h-3.5" style={{ color: 'var(--prism-4)' }} />,
					},
					{
						label: 'Risk Score',
						value: `${riskScore}`,
						icon: (
							<ShieldAlert
								className="w-3.5 h-3.5"
								style={{ color: isHighRisk ? 'var(--danger)' : 'var(--success)' }}
							/>
						),
					},
				].map(({ label, value, icon }) => (
					<div
						key={label}
						className="p-2.5 rounded-xl space-y-1"
						style={{ background: 'var(--white)', border: '1px solid var(--border)' }}
					>
						<div
							className="flex items-center justify-between text-[10px]"
							style={{ color: 'var(--ink-3)' }}
						>
							<span>{label}</span>
							{icon}
						</div>
						<p
							className="font-semibold font-mono text-xs"
							style={{
								color:
									label === 'Risk Score'
										? isHighRisk
											? 'var(--danger)'
											: 'var(--success)'
										: 'var(--ink)',
							}}
						>
							{value}
						</p>
					</div>
				))}
			</div>

			{/* Risk flags */}
			{safeFlags.length > 0 && (
				<div
					className="p-3 rounded-xl space-y-2"
					style={{
						background: 'rgba(255,77,79,0.04)',
						border: '1px solid rgba(255,77,79,0.16)',
					}}
				>
					<span
						className="text-[10px] font-bold uppercase tracking-wider block"
						style={{ color: 'var(--danger)' }}
					>
						Risk Factors ({safeFlags.length})
					</span>
					<div className="space-y-1.5">
						{safeFlags.map((flag: RiskFlag) => (
							<div
								key={flag.ruleId}
								className="p-2 rounded-lg text-[10px] space-y-0.5"
								style={{
									background: 'rgba(255,77,79,0.06)',
									border: '1px solid rgba(255,77,79,0.12)',
								}}
							>
								<div
									className="flex items-center justify-between font-medium"
									style={{ color: 'var(--ink)' }}
								>
									<span>{flag.ruleName}</span>
									<span style={{ color: 'var(--danger)' }}>+{flag.scoreImpact}</span>
								</div>
								<p style={{ color: 'var(--ink-3)' }}>{flag.description}</p>
							</div>
						))}
					</div>
				</div>
			)}
		</div>
	);
};
