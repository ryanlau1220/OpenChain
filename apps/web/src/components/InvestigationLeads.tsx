import { ClipboardCheck } from 'lucide-react';
import type React from 'react';
import type { GraphEdge, InvestigationLead } from '../services/api';

type RuleParameters = { window_seconds?: unknown; min_distinct_counterparties?: unknown };

export function leadThreshold(lead: InvestigationLead): string {
	try {
		const parameters = JSON.parse(lead.parametersJson) as RuleParameters;
		const hours =
			typeof parameters.window_seconds === 'number'
				? `${parameters.window_seconds / 3600}h window`
				: '';
		const counterparties =
			typeof parameters.min_distinct_counterparties === 'number'
				? `${parameters.min_distinct_counterparties} counterparties`
				: '';
		return [hours, counterparties].filter(Boolean).join(' · ') || 'Versioned rule parameters';
	} catch {
		return 'Versioned rule parameters';
	}
}

export const InvestigationLeads: React.FC<{
	leads: readonly InvestigationLead[];
	edges: readonly GraphEdge[];
	selectedLeadId?: string;
	onSelect: (lead: InvestigationLead) => void;
}> = ({ leads, edges, selectedLeadId, onSelect }) => {
	if (leads.length === 0) return null;
	const edgesByID = new Map(edges.map((edge) => [edge.id, edge]));
	return (
		<section className="space-y-2 border-t pt-4" style={{ borderColor: 'var(--border)' }}>
			<div className="flex items-center gap-1.5">
				<ClipboardCheck className="w-3.5 h-3.5" style={{ color: 'var(--accent)' }} />
				<h3
					className="text-[10px] uppercase font-bold tracking-widest"
					style={{ color: 'var(--ink-3)' }}
				>
					Investigation findings
				</h3>
			</div>
			<p className="text-[10px]" style={{ color: 'var(--ink-3)' }}>
				Rule-derived patterns from finalized transfers within the retrieved graph scope; not
				verified entity labels or investigator annotations.
			</p>
			{leads.map((lead) => {
				const evidence = lead.transferIds.map((id) => edgesByID.get(id)).filter(Boolean);
				const sources = [...new Set(evidence.map((edge) => edge?.sourceName).filter(Boolean))];
				const selected = lead.id === selectedLeadId;
				return (
					<button
						key={lead.id}
						type="button"
						onClick={() => onSelect(lead)}
						aria-pressed={selected}
						className="w-full rounded-lg p-2.5 text-left transition"
						style={{
							background: selected ? 'rgba(136,125,255,0.10)' : 'var(--white)',
							border: selected ? '1px solid rgba(136,125,255,0.50)' : '1px solid var(--border)',
						}}
					>
						<div className="flex items-start justify-between gap-2">
							<p className="text-xs font-semibold" style={{ color: 'var(--ink)' }}>
								{lead.title}
							</p>
							<span className="shrink-0 font-mono text-[9px]" style={{ color: 'var(--accent)' }}>
								v{lead.ruleVersion}
							</span>
						</div>
						<p className="mt-1 text-[10px]" style={{ color: 'var(--ink-2)' }}>
							{lead.rationale}
						</p>
						<p className="mt-1.5 font-mono text-[9px]" style={{ color: 'var(--accent)' }}>
							{leadThreshold(lead)} · {evidence.length}/{lead.transferIds.length} loaded transfers
						</p>
						{sources.length > 0 && (
							<p className="mt-1 text-[9px]" style={{ color: 'var(--ink-3)' }}>
								Source: {sources.join(', ')}
							</p>
						)}
						<p className="mt-1.5 text-[9px]" style={{ color: 'var(--ink-3)' }}>
							{lead.limitations}
						</p>
					</button>
				);
			})}
		</section>
	);
};
