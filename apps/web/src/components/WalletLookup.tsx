import { Clock, ExternalLink, ShieldAlert, Tag, Wallet } from 'lucide-react';
import type React from 'react';
import type {
	AddressSummary,
	LabelItem,
	RiskEvaluation,
} from '../services/api';

interface WalletLookupProps {
	summary: AddressSummary | null;
	risk: RiskEvaluation | null;
	labels: LabelItem[];
	loading: boolean;
	onTraceAddress: (addr: string) => void;
}

export const WalletLookup: React.FC<WalletLookupProps> = ({
	summary,
	risk,
	labels,
	loading,
	onTraceAddress,
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
				Select a node on the canvas to inspect wallet details and transaction
				history.
			</div>
		);
	}

	return (
		<div className="space-y-4 text-xs">
			{/* Address Header Banner */}
			<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1.5">
				<div className="flex items-center justify-between text-[11px] text-slate-400">
					<span className="font-semibold text-blue-400 uppercase">
						Target Address
					</span>
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
				{labels && labels.length > 0 && (
					<div className="flex flex-wrap gap-1 pt-1">
						{labels.map((l) => (
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
					<p className="font-bold font-mono text-slate-100 text-sm">
						{summary.balance_formatted}
					</p>
				</div>

				<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1">
					<div className="flex items-center justify-between text-slate-400 text-[11px]">
						<span>Tx Count</span>
						<Clock className="w-3.5 h-3.5 text-blue-400" />
					</div>
					<p className="font-bold font-mono text-slate-100 text-sm">
						{summary.tx_count}
					</p>
				</div>

				<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1">
					<div className="flex items-center justify-between text-slate-400 text-[11px]">
						<span>Entity</span>
						<Tag className="w-3.5 h-3.5 text-purple-400" />
					</div>
					<p className="font-bold text-slate-100 uppercase text-xs">
						{summary.entity_type}
					</p>
				</div>

				<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-1">
					<div className="flex items-center justify-between text-slate-400 text-[11px]">
						<span>Risk Level</span>
						<ShieldAlert className="w-3.5 h-3.5 text-amber-400" />
					</div>
					<p
						className={`font-bold font-mono text-xs ${summary.risk_score >= 50 ? 'text-red-400' : 'text-emerald-400'}`}
					>
						{summary.risk_level} ({summary.risk_score})
					</p>
				</div>
			</div>

			{/* Risk Flags List */}
			{risk && risk.flags.length > 0 && (
				<div className="p-3 bg-slate-950 border border-slate-800 rounded-lg space-y-2">
					<span className="font-semibold text-slate-400 text-[11px] block">
						Triggered Risk Factors ({risk.flags.length})
					</span>
					<div className="space-y-1.5">
						{risk.flags.map((flag) => (
							<div
								key={flag.rule_id}
								className="p-2 bg-slate-900 rounded border border-slate-800 text-[11px] space-y-0.5"
							>
								<div className="flex items-center justify-between font-medium text-slate-200">
									<span>{flag.rule_name}</span>
									<span className="text-red-400 font-mono">
										+{flag.score_impact}
									</span>
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
