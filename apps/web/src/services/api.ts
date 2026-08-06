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
	entity_type: string; // EOA, CONTRACT, EXCHANGE, MIXER, SCAMMER
	risk_score: number;
	category: string;
	is_seed: boolean;
	total_volume_wei: string;
	in_tx_count?: number;
	out_tx_count?: number;
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
	seed_addresses?: string[];
	seed_address?: string;
	nodes: GraphNode[];
	edges: GraphEdge[];
	total_nodes: number;
	total_edges: number;
}

export interface CanvasShareResponse {
	share_id: string;
	graph_data: GraphData;
	expires_at: string;
}

export async function lookupAddress(address: string) {
	const res = await fetch(
		`${API_BASE}/api/v1/lookup/address?address=${encodeURIComponent(address)}`,
	);
	if (!res.ok) throw new Error('Failed to lookup address');
	const data = await res.json();
	return {
		summary: data.summary,
		labels: Array.isArray(data.labels) ? data.labels : [],
		risk: {
			...data.risk,
			flags: Array.isArray(data.risk?.flags) ? data.risk.flags : [],
		},
	} as {
		summary: AddressSummary;
		labels: LabelItem[];
		risk: RiskEvaluation;
	};
}

export async function fetchTraceGraph(
	seedAddress: string,
	maxHops = 2,
	direction = 'BOTH',
) {
	return fetchMultiTraceGraph([seedAddress], maxHops, direction, []);
}

export async function fetchMultiTraceGraph(
	addresses: string[],
	maxHops = 2,
	direction = 'BOTH',
	tokens: string[] = [],
) {
	const res = await fetch(`${API_BASE}/api/v1/tracing/graph`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			addresses,
			max_hops: maxHops,
			direction,
			tokens,
		}),
	});
	if (!res.ok) throw new Error('Failed to fetch multi-trace graph');
	const data = await res.json();
	return {
		...data,
		nodes: Array.isArray(data.nodes) ? data.nodes : [],
		edges: Array.isArray(data.edges) ? data.edges : [],
	} as GraphData;
}

export async function shareCanvas(graphData: GraphData) {
	const res = await fetch(`${API_BASE}/api/v1/canvas/share`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(graphData),
	});
	if (!res.ok) throw new Error('Failed to share canvas');
	return res.json() as Promise<CanvasShareResponse>;
}

export async function getSharedCanvas(shareId: string) {
	const res = await fetch(
		`${API_BASE}/api/v1/canvas/share?share_id=${encodeURIComponent(shareId)}`,
	);
	if (!res.ok) throw new Error('Shared canvas expired or not found');
	const resData = await res.json();
	return {
		...resData,
		graph_data: {
			...resData.graph_data,
			nodes: Array.isArray(resData.graph_data?.nodes)
				? resData.graph_data.nodes
				: [],
			edges: Array.isArray(resData.graph_data?.edges)
				? resData.graph_data.edges
				: [],
		},
	} as CanvasShareResponse;
}
