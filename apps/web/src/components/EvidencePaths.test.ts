import { describe, expect, it } from 'vitest';
import type { GraphEdge, GraphNode } from '../services/api';
import { evidencePaths, orderedEvidencePaths } from './EvidencePaths';

describe('evidencePaths', () => {
	it('only reports direct labelled nodes with their observed transfer IDs', () => {
		const paths = evidencePaths(
			[
				{ id: '0x1', labels: [{ id: 'label-1', label: 'Known router' }] },
				{ id: '0x2', labels: [] },
			] as GraphNode[],
			[
				{ id: 'ethereum-mainnet:0xabc:0', source: '0x1', target: '0x2', transactionHash: '0xabc' },
			] as GraphEdge[],
		);
		expect(paths).toHaveLength(1);
		expect(paths[0]).toMatchObject({
			label: 'Known router',
			transferId: 'ethereum-mainnet:0xabc:0',
			transactionHash: '0xabc',
			asset: 'Unknown asset',
			amount: 'Unknown amount raw units',
			provenance: 'Unknown provider · retrieved Unknown observation time',
		});
	});

	it('keeps pinned paths ahead of new expansion evidence', () => {
		const paths = evidencePaths(
			[{ id: '0x1', labels: [{ id: 'label-1', label: 'Known service' }] }] as GraphNode[],
			[
				{ id: 'old', source: '0x1', target: '0x2', transactionHash: '0xold' },
				{ id: 'new', source: '0x1', target: '0x3', transactionHash: '0xnew' },
			] as GraphEdge[],
		);
		expect(orderedEvidencePaths(paths, ['old']).map((path) => path.transferId)).toEqual([
			'old',
			'new',
		]);
	});
});
