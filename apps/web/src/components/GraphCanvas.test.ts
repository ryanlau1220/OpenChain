import { describe, expect, it } from 'vitest';
import type { GraphEdge } from '../services/api';
import { aggregateGraphEdges } from './GraphCanvas';

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
	});
});
