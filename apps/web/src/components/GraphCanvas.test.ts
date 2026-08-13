import { describe, expect, it } from 'vitest';
import { type GraphEdge, TraceDirection } from '../services/api';
import {
	aggregateGraphEdges,
	filterGraphEdges,
	flowNodePositions,
	positionAddedNodes,
} from './GraphCanvas';

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

describe('positionAddedNodes', () => {
	it('keeps existing coordinates and places new neighbours around their parent', () => {
		const positions = positionAddedNodes(
			['new-a', 'new-b'],
			[
				{ source: 'seed', target: 'new-a' },
				{ source: 'seed', target: 'new-b' },
			],
			[{ id: 'seed', x: 24, y: 48 }],
		);
		expect(positions.get('seed')).toEqual({ x: 24, y: 48 });
		expect(positions.get('new-a')).not.toEqual(positions.get('new-b'));
	});

	it('adds inbound and outbound neighbours to the correct empty flow column', () => {
		const positions = positionAddedNodes(
			['source', 'destination'],
			[
				{ source: 'source', target: 'seed' },
				{ source: 'seed', target: 'destination' },
			],
			[{ id: 'seed', x: 0, y: 0 }],
			true,
		);
		expect(positions.get('source')?.x).toBeLessThan(0);
		expect(positions.get('destination')?.x).toBeGreaterThan(0);
	});
});

describe('flowNodePositions', () => {
	it('places funding sources left of the target and destinations right', () => {
		const positions = flowNodePositions(
			'seed',
			['source', 'seed', 'destination'],
			[
				{ source: 'source', target: 'seed' },
				{ source: 'seed', target: 'destination' },
			],
			TraceDirection.BOTH,
		);
		expect(positions.get('source')?.x).toBeLessThan(positions.get('seed')?.x ?? 0);
		expect(positions.get('destination')?.x).toBeGreaterThan(positions.get('seed')?.x ?? 0);
	});
});
