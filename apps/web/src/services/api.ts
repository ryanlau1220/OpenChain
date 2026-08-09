import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import type { AddressSummary } from '@openchain/proto/openchain/v1/common_pb';
import { EntityType } from '@openchain/proto/openchain/v1/common_pb';
import { LabelService } from '@openchain/proto/openchain/v1/labels_connect';
import type { AddressLabel } from '@openchain/proto/openchain/v1/labels_pb';
import { LookupService } from '@openchain/proto/openchain/v1/lookup_connect';
import { TracingService } from '@openchain/proto/openchain/v1/tracing_connect';
import {
	type GraphEdge,
	type GraphNode,
	TraceGraphResponse,
} from '@openchain/proto/openchain/v1/tracing_pb';

const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8081';

const transport = createConnectTransport({ baseUrl: API_BASE });

export const tracingClient = createClient(TracingService, transport);
export const lookupClient = createClient(LookupService, transport);
export const labelClient = createClient(LabelService, transport);

// Re-export generated proto types for consumer convenience
export type {
	GraphNode,
	GraphEdge,
	AddressLabel,
	AddressSummary,
};
export { EntityType, TraceGraphResponse };

export function entityLabel(t?: EntityType): string {
	switch (t) {
		case EntityType.CONTRACT:
			return 'CONTRACT';
		case EntityType.EXCHANGE:
			return 'EXCHANGE';
		case EntityType.MIXER:
			return 'MIXER';
		case EntityType.BRIDGE:
			return 'BRIDGE';
		case EntityType.DEFI_POOL:
			return 'DEFI POOL';
		default:
			return 'EOA';
	}
}

// ── Tracing ─────────────────────────────────────────────────────────────────────

export async function fetchTraceGraph(seedAddress: string): Promise<TraceGraphResponse> {
	return tracingClient.traceGraph({ seedAddress, network: 1 });
}

export async function lookupAddress(address: string) {
	const [lookupRes, labelsRes] = await Promise.allSettled([
		lookupClient.lookupAddress({
			address,
			network: 1 /* NETWORK_ETHEREUM_SEPOLIA */,
		}),
		labelClient.getLabels({ address, network: 1 }),
	]);
	return {
		summary: lookupRes.status === 'fulfilled' ? lookupRes.value.summary : undefined,
		labels: labelsRes.status === 'fulfilled' ? labelsRes.value.labels : [],
	};
}

export async function expandNode(address: string) {
	return tracingClient.expandNode({ nodeAddress: address, network: 1 });
}
