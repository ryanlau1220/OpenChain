import { Clock, ExternalLink, ShieldAlert, Tag, Wallet } from 'lucide-react';
import type React from 'react';
import {
	type AddressLabel,
	type AddressSummary,
	type RiskEvaluation,
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
			<div className="p-6 flex flex-col items-center justify-center text-slate-400 gap-3">
				<div className="w-6 h-6 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
				<p className="text-xs">Querying EVM RPC...</p>
			</div>
		);
	}

	if (!summary) {
		return (
			<div className="p-6 text-center text-slate-500 text-xs">
				Select a node on the canvas to inspect wallet details and transaction history.
			</div>
		);
	}

	const safeLabels = Array.isArray(labels) ? labels : [];
	const safeFlags = Array.isArray(risk?.flags) ? risk.flags : [];

	return (
		<div className="space-y-4 text-xs">
			{/* Address Header Banner */}
			<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1.5">
				<div className="flex items-center justify-between text-[11px] text-slate-400">
					<span className="font-semibold text-blue-400 uppercase">Target Address</span>
					<a
						href={`https://sepolia.etherscan.io/address/${summary.address}`}
						target="_blank"
						rel="noreferrer"
						className="text-slate-400 hover:text-white transition flex items-center gap-1"
					>
						Etherscan <ExternalLink className="w-3 h-3" />
					</a>
				</div>
				<p className="font-mono font-bold text-slate-100 break-all select-all text-[11px]">
					{summary.address}
				</p>
				{safeLabels.length > 0 && (
					<div className="flex flex-wrap gap-1 pt-1">
						{safeLabels.map((l) => (
							<span
								key={l.id}
								className="text-[10px] uppercase font-semibold px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-300 border border-blue-500/20"
							>
								{l.category}: {l.label}
							</span>
						))}
					</div>
				)}
			</div>

			{/* Metric Grid */}
			<div className="grid grid-cols-2 gap-2">
				<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1">
					<div className="flex items-center justify-between text-slate-400 text-[11px]">
						<span>Balance</span>
						<Wallet className="w-3.5 h-3.5 text-blue-400" />
					</div>
					<p className="font-bold font-mono text-slate-100 text-sm">{summary.balanceFormatted}</p>
				</div>

				<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1">
					<div className="flex items-center justify-between text-slate-400 text-[11px]">
						<span>Tx Count</span>
						<Clock className="w-3.5 h-3.5 text-blue-400" />
					</div>
					<p className="font-bold font-mono text-slate-100 text-sm">{summary.txCount}</p>
				</div>

				<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1">
					<div className="flex items-center justify-between text-slate-400 text-[11px]">
						<span>Entity</span>
						<Tag className="w-3.5 h-3.5 text-purple-400" />
					</div>
					<p className="font-bold text-slate-100 uppercase text-xs">
						{entityLabel(summary.entityType)}
					</p>
				</div>

				<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1">
					<div className="flex items-center justify-between text-slate-400 text-[11px]">
						<span>Risk Level</span>
						<ShieldAlert className="w-3.5 h-3.5 text-amber-400" />
					</div>
					<p
						className={`font-bold font-mono text-xs ${(risk?.totalScore ?? 0) >= 50 ? 'text-red-400' : 'text-emerald-400'}`}
					>
						{risk?.riskLevel ?? 'N/A'} ({risk?.totalScore ?? 0})
					</p>
				</div>
			</div>

			{/* Risk Flags List */}
			{safeFlags.length > 0 && (
				<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-2">
					<span className="font-semibold text-slate-400 text-[11px] block">
						Triggered Risk Factors ({safeFlags.length})
					</span>
					<div className="space-y-1.5">
						{safeFlags.map((flag) => (
							<div
								key={flag.ruleId}
								className="p-2 bg-slate-900 rounded border border-slate-800 text-[11px] space-y-0.5"
							>
								<div className="flex items-center justify-between font-medium text-slate-200">
									<span>{flag.ruleName}</span>
									<span className="text-red-400 font-mono">+{flag.scoreImpact}</span>
								</div>
								<p className="text-slate-400 text-[10px]">{flag.description}</p>
							</div>
						))}
					</div>
				</div>
			)}
		</div>
	);
};
