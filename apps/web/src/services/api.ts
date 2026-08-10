import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import type { AddressSummary } from '@openchain/proto/openchain/v1/common_pb';
import { EntityType } from '@openchain/proto/openchain/v1/common_pb';
import { LabelService } from '@openchain/proto/openchain/v1/labels_connect';
import { LabelVisibility, type AddressLabel } from '@openchain/proto/openchain/v1/labels_pb';
import { LookupService } from '@openchain/proto/openchain/v1/lookup_connect';
import { TracingService } from '@openchain/proto/openchain/v1/tracing_connect';
import {
	type GraphEdge,
	type GraphNode,
	TraceGraphResponse,
} from '@openchain/proto/openchain/v1/tracing_pb';

const API_BASE = import.meta.env.VITE_API_URL || (typeof window === 'undefined' ? '' : window.location.origin);

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
export { EntityType, LabelVisibility, TraceGraphResponse };

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

export async function fetchTraceGraph(seedAddress: string, retry = false): Promise<TraceGraphResponse> {
	return tracingClient.traceGraph({ seedAddress, network: 1, limit: 25, retry });
}

export async function lookupAddress(address: string) {
	const [lookupRes, labelsRes] = await Promise.allSettled([
		lookupClient.lookupAddress({
			address,
			network: 1 /* NETWORK_ETHEREUM_MAINNET */,
		}),
		labelClient.getLabels({ address, network: 1 }),
	]);
	return {
		summary: lookupRes.status === 'fulfilled' ? lookupRes.value.summary : undefined,
		labels: labelsRes.status === 'fulfilled' ? labelsRes.value.labels : [],
	};
}

export async function expandNode(address: string, cursor = '', retry = false) {
	return tracingClient.expandNode({ nodeAddress: address, network: 1, limit: 25, cursor, retry });
}

export function requestErrorMessage(error: unknown, fallback: string): string {
	if (error instanceof ConnectError && error.code === Code.ResourceExhausted) return error.rawMessage;
	if (error instanceof ConnectError && error.code === Code.Unavailable) return 'Ethereum data is temporarily unavailable. Please try again.';
	return fallback;
}
