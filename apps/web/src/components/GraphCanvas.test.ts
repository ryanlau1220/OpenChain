import { describe, expect, it } from 'vitest';
import type { GraphEdge } from '../services/api';
import { aggregateGraphEdges, filterGraphEdges } from './GraphCanvas';

describe('aggregateGraphEdges', () => {
	it('combines only transfers with the same direction and asset', () => {
		const edges = [
			{
				id: '1',
				source: 'from',
				target: 'to',
				amountBaseUnits: '1200000',
				txCount: 1,
				asset: { kind: 'ERC20', contractAddress: 'usdc', symbol: 'USDC', decimals: 6 },
				lastTxTimestamp: 1n,
			},
			{
				id: '2',
				source: 'from',
				target: 'to',
				amountBaseUnits: '800000',
				txCount: 1,
				asset: { kind: 'ERC20', contractAddress: 'usdc', symbol: 'USDC', decimals: 6 },
				lastTxTimestamp: 2n,
			},
			{
				id: '3',
				source: 'to',
				target: 'from',
				amountBaseUnits: '1',
				txCount: 1,
				asset: { kind: 'NATIVE', symbol: 'ETH', decimals: 18 },
				lastTxTimestamp: 3n,
			},
		] as GraphEdge[];

		const relationships = aggregateGraphEdges(edges);
		expect(relationships).toHaveLength(2);
		expect(relationships[0].label).toBe('2 transfers · 2 USDC');
		expect(relationships[0].representative.id).toBe('2');
		expect(relationships[0].transfers.map((edge) => edge.id)).toEqual(['1', '2']);
	});

	it('filters by target direction, asset amount, type, and inclusive date range', () => {
		const edges = [
			{
				id: 'inbound',
				source: 'from',
				target: 'seed',
				amountBaseUnits: '1500000',
				asset: { kind: 'ERC20', contractAddress: 'usdc', symbol: 'USDC', decimals: 6 },
				transferKind: 'ERC20',
				firstTxTimestamp: 1_704_067_200n,
			},
			{
				id: 'outbound',
				source: 'seed',
				target: 'to',
				amountBaseUnits: '2',
				asset: { kind: 'NATIVE', symbol: 'ETH', decimals: 0 },
				transferKind: 'NATIVE',
				firstTxTimestamp: 1_704_153_600n,
			},
		] as GraphEdge[];
		const visible = filterGraphEdges(edges, 'seed', {
			from: '2024-01-01',
			to: '2024-01-01',
			direction: 'inbound',
			asset: 'ERC20:usdc:6',
			minimumAmount: '1.5',
			transferKind: 'ERC20',
		});
		expect(visible.map((edge) => edge.id)).toEqual(['inbound']);
	});
});
