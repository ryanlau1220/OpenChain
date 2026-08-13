import type { GraphEdge } from './api';

export function nodeDepths(seedAddress: string, edges: readonly GraphEdge[]): Map<string, number> {
	const neighbours = new Map<string, string[]>();
	for (const edge of edges) {
		neighbours.set(edge.source, [...(neighbours.get(edge.source) || []), edge.target]);
		neighbours.set(edge.target, [...(neighbours.get(edge.target) || []), edge.source]);
	}
	const depths = new Map<string, number>([[seedAddress, 0]]);
	const queue = [seedAddress];
	for (let index = 0; index < queue.length; index++) {
		const current = queue[index];
		for (const neighbour of neighbours.get(current) || []) {
			if (depths.has(neighbour)) continue;
			depths.set(neighbour, (depths.get(current) || 0) + 1);
			queue.push(neighbour);
		}
	}
	return depths;
}
