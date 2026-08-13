import { describe, expect, it } from 'vitest';
import type { GraphEdge } from './api';
import { nodeDepths } from './graph-scope';

describe('nodeDepths', () => {
	it('uses the shortest path from the target and remains finite for cycles', () => {
		const depths = nodeDepths('seed', [
			{ source: 'seed', target: 'one' },
			{ source: 'one', target: 'two' },
			{ source: 'two', target: 'seed' },
		] as GraphEdge[]);
		expect(depths.get('seed')).toBe(0);
		expect(depths.get('one')).toBe(1);
		expect(depths.get('two')).toBe(1);
	});
});
