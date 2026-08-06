const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8081';

export interface AddressSummary {
	address: string;
	network: string;
	entity_type: string;
	label: string;
	balance_wei: string;
	balance_formatted: string;
	tx_count: number;
	risk_score: number;
	risk_level: string;
}

export interface LabelItem {
	id: string;
	address: string;
	network: string;
	category: string;
	label: string;
	confidence: number;
	evidence_url: string;
	source: string;
	created_by: string;
	created_at: string;
}

export interface RiskFlag {
	rule_id: string;
	rule_name: string;
	severity: string;
	score_impact: number;
	description: string;
	evidence_detail: string;
}

export interface RiskEvaluation {
	address: string;
	network: string;
	total_score: number;
	risk_level: string;
	flags: RiskFlag[];
	evaluated_at: string;
}

export interface GraphNode {
	id: string;
	label: string;
	entity_type: string;
	risk_score: number;
	category: string;
	is_seed: boolean;
	total_volume_wei: string;
}

export interface GraphEdge {
	id: string;
	source: string;
	target: string;
	value_wei: string;
	value_formatted: string;
	tx_count: number;
	asset_symbol: string;
}

export interface GraphData {
	seed_address: string;
	nodes: GraphNode[];
	edges: GraphEdge[];
	total_nodes: number;
	total_edges: number;
}

export interface InvestigationCase {
	id: string;
	title: string;
	description: string;
	status: string;
	tags: string[];
	created_by: string;
	created_at: string;
}

export async function lookupAddress(address: string) {
	const res = await fetch(
		`${API_BASE}/api/v1/lookup/address?address=${encodeURIComponent(address)}`,
	);
	if (!res.ok) throw new Error('Failed to lookup address');
	return res.json() as Promise<{
		summary: AddressSummary;
		labels: LabelItem[];
		risk: RiskEvaluation;
	}>;
}

export async function fetchTraceGraph(seedAddress: string, maxHops = 2) {
	const res = await fetch(
		`${API_BASE}/api/v1/tracing/graph?seed_address=${encodeURIComponent(seedAddress)}&max_hops=${maxHops}`,
	);
	if (!res.ok) throw new Error('Failed to fetch trace graph');
	return res.json() as Promise<GraphData>;
}

export async function addLabel(label: Partial<LabelItem>) {
	const res = await fetch(`${API_BASE}/api/v1/labels`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(label),
	});
	if (!res.ok) throw new Error('Failed to add label');
	return res.json() as Promise<LabelItem>;
}

export async function fetchCases() {
	const res = await fetch(`${API_BASE}/api/v1/cases`);
	if (!res.ok) throw new Error('Failed to fetch cases');
	return res.json() as Promise<InvestigationCase[]>;
}

export async function createCase(data: {
	title: string;
	description: string;
	tags: string[];
}) {
	const res = await fetch(`${API_BASE}/api/v1/cases`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(data),
	});
	if (!res.ok) throw new Error('Failed to create case');
	return res.json() as Promise<InvestigationCase>;
}

export function getExportUrl(caseId: string, format = 'JSON') {
	return `${API_BASE}/api/v1/cases/export?case_id=${encodeURIComponent(caseId)}&format=${format}`;
}
