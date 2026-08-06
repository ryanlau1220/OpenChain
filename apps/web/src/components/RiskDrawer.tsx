import { ShieldAlert } from 'lucide-react';
import type React from 'react';

export const RiskDrawer: React.FC = () => {
	const rules = [
		{
			id: 'R001',
			name: 'Sanction List Entity Exposure',
			severity: 'CRITICAL',
			score: 90,
			desc: 'Triggers when address is flagged in OFAC / EU sanction databases.',
		},
		{
			id: 'R002',
			name: 'Privacy Mixer Interaction',
			severity: 'HIGH',
			score: 50,
			desc: 'Triggers when funds pass through Tornado Cash or privacy pools.',
		},
		{
			id: 'R003',
			name: 'Exploit & Hack Associated',
			severity: 'HIGH',
			score: 70,
			desc: 'Triggers when address is linked to known protocol exploits.',
		},
		{
			id: 'R004',
			name: 'High-Frequency Contract Velocity',
			severity: 'MEDIUM',
			score: 15,
			desc: 'Triggers when a contract executes > 1,000 automated transactions.',
		},
		{
			id: 'R005',
			name: 'Unused / Fresh Address',
			severity: 'LOW',
			score: 5,
			desc: 'Triggers when an address has zero recorded transactions.',
		},
	];

	return (
		<div className="p-8 max-w-5xl mx-auto space-y-6">
			<div>
				<h2 className="text-xl font-bold text-white font-sans flex items-center gap-2">
					<ShieldAlert className="w-5 h-5 text-amber-400" />
					Rule-Based Risk Indicators Engine
				</h2>
				<p className="text-xs text-slate-400">
					Deterministic, explainable risk rules (No black-box AI predictions).
				</p>
			</div>

			<div className="grid grid-cols-1 gap-4">
				{rules.map((rule) => (
					<div
						key={rule.id}
						className="p-5 glass-panel rounded-2xl flex items-center justify-between border-l-4 border-amber-500"
					>
						<div className="space-y-1">
							<div className="flex items-center gap-2">
								<span className="font-mono font-bold text-xs text-amber-400">
									{rule.id}
								</span>
								<h3 className="font-bold text-sm text-slate-100">
									{rule.name}
								</h3>
								<span className="text-[10px] font-bold uppercase px-2 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20">
									{rule.severity}
								</span>
							</div>
							<p className="text-xs text-slate-400">{rule.desc}</p>
						</div>
						<div className="text-right">
							<span className="text-lg font-mono font-bold text-slate-200">
								+{rule.score}
							</span>
							<p className="text-[10px] text-slate-500 uppercase">
								Impact Score
							</p>
						</div>
					</div>
				))}
			</div>
		</div>
	);
};
