import { describe, expect, it } from 'vitest';
import { evidencePaths } from './EvidencePaths';
import type { GraphEdge, GraphNode } from '../services/api';

describe('evidencePaths', () => {
	it('only reports direct labelled nodes with their observed transfer IDs', () => {
		const paths = evidencePaths(
			[{ id: '0x1', labels: [{ id: 'label-1', label: 'Known router' }] }, { id: '0x2', labels: [] }] as GraphNode[],
			[{ id: 'ethereum-mainnet:0xabc:0', source: '0x1', target: '0x2', transactionHash: '0xabc' }] as GraphEdge[],
		);
		expect(paths).toEqual([{ label: 'Known router', transferId: 'ethereum-mainnet:0xabc:0', transactionHash: '0xabc' }]);
	});
});
