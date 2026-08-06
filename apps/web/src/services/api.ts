import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { CanvasService } from '@openchain/proto/openchain/v1/canvas_connect';
import type { CanvasSnapshot } from '@openchain/proto/openchain/v1/canvas_pb';
import { CaseService } from '@openchain/proto/openchain/v1/cases_connect';
import type { InvestigationCase } from '@openchain/proto/openchain/v1/cases_pb';
import type { AddressSummary } from '@openchain/proto/openchain/v1/common_pb';
import { EntityType } from '@openchain/proto/openchain/v1/common_pb';
import { LabelService } from '@openchain/proto/openchain/v1/labels_connect';
import type { AddressLabel } from '@openchain/proto/openchain/v1/labels_pb';
import { LookupService } from '@openchain/proto/openchain/v1/lookup_connect';
import { RiskService } from '@openchain/proto/openchain/v1/risk_connect';
import type { RiskEvaluation } from '@openchain/proto/openchain/v1/risk_pb';
import { TracingService } from '@openchain/proto/openchain/v1/tracing_connect';
import { TraceDirection } from '@openchain/proto/openchain/v1/tracing_pb';
import type {
	GraphEdge,
	GraphNode,
} from '@openchain/proto/openchain/v1/tracing_pb';

const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8081';

const transport = createConnectTransport({ baseUrl: API_BASE });

export const tracingClient = createClient(TracingService, transport);
export const lookupClient = createClient(LookupService, transport);
export const labelClient = createClient(LabelService, transport);
export const riskClient = createClient(RiskService, transport);
export const caseClient = createClient(CaseService, transport);
export const canvasClient = createClient(CanvasService, transport);

// Re-export generated proto types for consumer convenience
export type {
	GraphNode,
	GraphEdge,
	AddressLabel,
	RiskEvaluation,
	InvestigationCase,
	AddressSummary,
};
export { EntityType };

export function entityLabel(t?: EntityType): string {
	switch (t) {
		case EntityType.ENTITY_TYPE_CONTRACT:
			return 'CONTRACT';
		case EntityType.ENTITY_TYPE_EXCHANGE:
			return 'EXCHANGE';
		case EntityType.ENTITY_TYPE_MIXER:
			return 'MIXER';
		case EntityType.ENTITY_TYPE_BRIDGE:
			return 'BRIDGE';
		case EntityType.ENTITY_TYPE_DEFI_POOL:
			return 'DEFI POOL';
		default:
			return 'EOA';
	}
}

// GraphData is the view type used to build a CanvasSnapshot for sharing
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

// ── Tracing ─────────────────────────────────────────────────────────────────────

export async function fetchTraceGraph(
	seedAddress: string,
	maxHops = 2,
	direction = 'BOTH',
) {
	return fetchMultiTraceGraph([seedAddress], maxHops, direction, []);
}

export async function lookupAddress(address: string) {
	const [lookupResp, riskResp, labelsResp] = await Promise.all([
		lookupClient.lookupAddress({
			address,
			network: 1 /* NETWORK_ETHEREUM_SEPOLIA */,
		}),
		riskClient.evaluateRisk({ address, network: 1 }),
		labelClient.getLabels({ address, network: 1 }),
	]);
	return {
		summary: lookupResp.summary,
		risk: riskResp.evaluation,
		labels: labelsResp.labels,
	};
}

export async function fetchMultiTraceGraph(
	addresses: string[],
	maxHops = 2,
	direction = 'BOTH',
	tokens: string[] = [],
): Promise<GraphData> {
	const traceDir =
		direction === 'INFLOW'
			? TraceDirection.INBOUND
			: direction === 'OUTFLOW'
				? TraceDirection.OUTBOUND
				: TraceDirection.BOTH;

	const response = await tracingClient.traceGraph({
		seedAddresses: addresses,
		maxHops,
		direction: traceDir,
		tokens,
	});

	return {
		seed_addresses: addresses,
		seed_address: response.seedAddress,
		nodes: response.nodes as unknown as GraphNode[],
		edges: response.edges as unknown as GraphEdge[],
		total_nodes: response.totalNodes,
		total_edges: response.totalEdges,
	};
}

// ── Canvas Share ───────────────────────────────────────────────────────────────

export async function shareCanvas(graphData: GraphData) {
	const snapshot: CanvasSnapshot = {
		seedAddress: graphData.seed_address ?? '',
		seedAddresses: graphData.seed_addresses ?? [],
		nodes: graphData.nodes,
		edges: graphData.edges,
		totalNodes: graphData.total_nodes,
		totalEdges: graphData.total_edges,
	};
	const res = await canvasClient.shareCanvas({ snapshot });
	return { share_id: res.shareId, expires_at: res.expiresAt };
}

export async function getSharedCanvas(shareId: string) {
	const res = await canvasClient.getSharedCanvas({ shareId });
	const s = res.snapshot;
	return {
		share_id: res.shareId,
		expires_at: res.expiresAt,
		graph_data: {
			seed_address: s?.seedAddress,
			seed_addresses: s?.seedAddresses ?? [],
			nodes: s?.nodes ?? [],
			edges: s?.edges ?? [],
			total_nodes: s?.totalNodes ?? 0,
			total_edges: s?.totalEdges ?? 0,
		},
	} as CanvasShareResponse;
}
