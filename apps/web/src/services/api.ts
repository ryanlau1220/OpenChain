import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import type { AddressSummary } from '@openchain/proto/openchain/v1/common_pb';
import { EntityType, Network } from '@openchain/proto/openchain/v1/common_pb';
import { EvidenceService } from '@openchain/proto/openchain/v1/evidence_connect';
import { LabelService } from '@openchain/proto/openchain/v1/labels_connect';
import { AddressLabel, LabelVisibility } from '@openchain/proto/openchain/v1/labels_pb';
import { LookupService } from '@openchain/proto/openchain/v1/lookup_connect';
import { TracingService } from '@openchain/proto/openchain/v1/tracing_connect';
import {
	GraphEdge,
	GraphNode,
	GraphRanking,
	InvestigationLead,
	TraceDirection,
	TraceGraphResponse,
} from '@openchain/proto/openchain/v1/tracing_pb';

const API_BASE =
	import.meta.env.VITE_API_URL || (typeof window === 'undefined' ? '' : window.location.origin);

const transport = createConnectTransport({ baseUrl: API_BASE });

export const tracingClient = createClient(TracingService, transport);
export const lookupClient = createClient(LookupService, transport);
export const labelClient = createClient(LabelService, transport);
export const evidenceClient = createClient(EvidenceService, transport);

// Re-export generated proto types for consumer convenience
export type { AddressSummary };
export {
	AddressLabel,
	EntityType,
	GraphEdge,
	GraphNode,
	GraphRanking,
	InvestigationLead,
	LabelVisibility,
	Network,
	TraceDirection,
	TraceGraphResponse,
};

export type SupportedNetwork =
	| Network.ETHEREUM_MAINNET
	| Network.BASE_MAINNET
	| Network.POLYGON_MAINNET
	| Network.ARBITRUM_ONE
	| Network.OPTIMISM_MAINNET
	| Network.BNB_CHAIN
	| Network.TON_MAINNET
	| Network.CARDANO_MAINNET
	| Network.SOLANA_MAINNET
	| Network.TRON_MAINNET;

export type NetworkSlug =
	| 'ethereum-mainnet'
	| 'base-mainnet'
	| 'polygon-mainnet'
	| 'arbitrum-one'
	| 'optimism-mainnet'
	| 'bnb-chain'
	| 'ton-mainnet'
	| 'cardano-mainnet'
	| 'solana-mainnet'
	| 'tron-mainnet';

const NETWORK_DETAILS: Record<
	SupportedNetwork,
	{
		name: string;
		slug: NetworkSlug;
		explorer: string;
		icon: string;
		activityLabel?: string;
	}
> = {
	[Network.ETHEREUM_MAINNET]: {
		name: 'Ethereum Mainnet',
		slug: 'ethereum-mainnet',
		explorer: 'https://etherscan.io',
		icon: '/networks/ethereum.svg',
		activityLabel: 'Outgoing nonce',
	},
	[Network.BASE_MAINNET]: {
		name: 'Base Mainnet',
		slug: 'base-mainnet',
		explorer: 'https://basescan.org',
		icon: '/networks/base.svg',
		activityLabel: 'Outgoing nonce',
	},
	[Network.POLYGON_MAINNET]: {
		name: 'Polygon Mainnet',
		slug: 'polygon-mainnet',
		explorer: 'https://polygonscan.com',
		icon: '/networks/polygon.svg',
		activityLabel: 'Outgoing nonce',
	},
	[Network.ARBITRUM_ONE]: {
		name: 'Arbitrum One',
		slug: 'arbitrum-one',
		explorer: 'https://arbiscan.io',
		icon: '/networks/arbitrum.svg',
		activityLabel: 'Outgoing nonce',
	},
	[Network.OPTIMISM_MAINNET]: {
		name: 'Optimism Mainnet',
		slug: 'optimism-mainnet',
		explorer: 'https://optimistic.etherscan.io',
		icon: '/networks/optimism.svg',
		activityLabel: 'Outgoing nonce',
	},
	[Network.BNB_CHAIN]: {
		name: 'BNB Chain',
		slug: 'bnb-chain',
		explorer: 'https://bscscan.com',
		icon: '/networks/bnb.svg',
		activityLabel: 'Outgoing nonce',
	},
	[Network.SOLANA_MAINNET]: {
		name: 'Solana Mainnet',
		slug: 'solana-mainnet',
		explorer: 'https://explorer.solana.com',
		icon: '/networks/solana.svg',
	},
	[Network.TRON_MAINNET]: {
		name: 'TRON Mainnet',
		slug: 'tron-mainnet',
		explorer: 'https://tronscan.org/#',
		icon: '/networks/tron.svg',
	},
	[Network.TON_MAINNET]: {
		name: 'TON Mainnet',
		slug: 'ton-mainnet',
		explorer: 'https://tonviewer.com',
		icon: '/networks/ton.svg',
	},
	[Network.CARDANO_MAINNET]: {
		name: 'Cardano Mainnet',
		slug: 'cardano-mainnet',
		explorer: 'https://cardanoscan.io',
		icon: '/networks/cardano.svg',
	},
};

export const supportedNetworks = [
	Network.ETHEREUM_MAINNET,
	Network.BASE_MAINNET,
	Network.POLYGON_MAINNET,
	Network.ARBITRUM_ONE,
	Network.OPTIMISM_MAINNET,
	Network.BNB_CHAIN,
	Network.SOLANA_MAINNET,
	Network.TRON_MAINNET,
	Network.TON_MAINNET,
	Network.CARDANO_MAINNET,
] as const;

export function networkDetails(network: SupportedNetwork) {
	return NETWORK_DETAILS[network];
}

export function networkFromSlug(slug: NetworkSlug): SupportedNetwork {
	return (
		supportedNetworks.find((network) => NETWORK_DETAILS[network].slug === slug) ??
		Network.ETHEREUM_MAINNET
	);
}

const BASE58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';

function decodedBase58Length(value: string): number {
	if (!/^[1-9A-HJ-NP-Za-km-z]+$/.test(value) || value.length > 64) return 0;
	let number = 0n;
	for (const character of value) number = number * 58n + BigInt(BASE58.indexOf(character));
	let bytes = 0;
	for (let current = number; current > 0n; current /= 256n) bytes++;
	return bytes + (value.length - value.replace(/^1+/, '').length);
}

export function detectAddressNetwork(value: string): SupportedNetwork | undefined {
	const address = value.trim();
	// An EVM address cannot identify a specific EVM chain. Default to Ethereum,
	// while leaving the network control available for an investigator to choose Base.
	if (/^0x[\da-fA-F]{40}$/.test(address)) return Network.ETHEREUM_MAINNET;
	if (/^T[1-9A-HJ-NP-Za-km-z]{33}$/.test(address) && decodedBase58Length(address) === 25)
		return Network.TRON_MAINNET;
	if ((address.startsWith('EQ') || address.startsWith('UQ')) && address.length === 48)
		return Network.TON_MAINNET;
	if ((address.startsWith('addr1') || address.startsWith('addr_test1')) && address.length >= 50)
		return Network.CARDANO_MAINNET;
	if (decodedBase58Length(address) === 32) return Network.SOLANA_MAINNET;
	return undefined;
}

export function isEVMNetwork(network: SupportedNetwork): boolean {
	return (
		network === Network.ETHEREUM_MAINNET ||
		network === Network.BASE_MAINNET ||
		network === Network.POLYGON_MAINNET ||
		network === Network.ARBITRUM_ONE ||
		network === Network.OPTIMISM_MAINNET ||
		network === Network.BNB_CHAIN
	);
}

export function explorerURL(network: SupportedNetwork, resource: 'address' | 'tx', value: string) {
	if (network === Network.TRON_MAINNET)
		return `${NETWORK_DETAILS[network].explorer}/${resource === 'tx' ? 'transaction' : 'address'}/${value}`;
	return `${NETWORK_DETAILS[network].explorer}/${resource}/${value}`;
}

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

export type GraphOptions = {
	maxCounterparties: number;
	ranking: GraphRanking;
	direction: TraceDirection;
};

export const defaultGraphOptions: GraphOptions = {
	maxCounterparties: 10,
	ranking: GraphRanking.MOST_RECENT,
	direction: TraceDirection.BOTH,
};

export async function fetchTraceGraph(
	seedAddress: string,
	network: SupportedNetwork,
	options: GraphOptions = defaultGraphOptions,
	retry = false,
): Promise<TraceGraphResponse> {
	return tracingClient.traceGraph({ seedAddress, network, limit: 50, retry, ...options });
}

export async function fetchTraceStatus(
	address: string,
	network: SupportedNetwork,
	cursor = '',
	options: GraphOptions = defaultGraphOptions,
): Promise<TraceGraphResponse> {
	return tracingClient.getTraceStatus({ address, network, limit: 50, cursor, ...options });
}

export async function lookupAddress(address: string, network: SupportedNetwork) {
	const [lookupRes, labelsRes] = await Promise.allSettled([
		lookupClient.lookupAddress({
			address,
			network,
		}),
		labelClient.getLabels({ address, network }),
	]);
	return {
		summary: lookupRes.status === 'fulfilled' ? lookupRes.value.summary : undefined,
		labels: labelsRes.status === 'fulfilled' ? labelsRes.value.labels : [],
	};
}

export async function expandNode(
	address: string,
	network: SupportedNetwork,
	cursor = '',
	options: GraphOptions = defaultGraphOptions,
	retry = false,
) {
	return tracingClient.expandNode({
		nodeAddress: address,
		network,
		limit: 50,
		cursor,
		retry,
		...options,
	});
}

export async function exportEvidencePackage(
	network: SupportedNetwork,
	transferIds: readonly string[],
	caseJSON: string,
): Promise<string> {
	const response = await evidenceClient.exportEvidencePackage({
		network,
		transferIds: [...transferIds],
		caseJson: caseJSON,
	});
	return response.packageJson;
}

export function requestErrorMessage(error: unknown, fallback: string): string {
	if (error instanceof ConnectError && error.code === Code.ResourceExhausted)
		return error.rawMessage;
	if (error instanceof ConnectError && error.code === Code.Unavailable)
		return 'Blockchain data is temporarily unavailable. Please try again.';
	return fallback;
}
