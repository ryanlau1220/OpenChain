import { type GraphOptions, type NetworkSlug, defaultGraphOptions, isNetworkSlug } from './api';

export const CASE_FILE_VERSION = 2;
const STORAGE_KEY = 'openchain.local-case.v2';

export type CaseAnnotation = {
	id: string;
	target: { kind: 'address' | 'transfer'; id: string };
	note: string;
	createdAt: string;
};

export type LocalCase = {
	version: number;
	title: string;
	network: NetworkSlug;
	scope: GraphOptions;
	createdAt: string;
	updatedAt: string;
	rootAddress: string;
	notes: string;
	selectedAddressIds: string[];
	selectedTransferIds: string[];
	annotations: CaseAnnotation[];
};

export function createLocalCase(): LocalCase {
	const now = new Date().toISOString();
	return {
		version: CASE_FILE_VERSION,
		title: 'Untitled investigation',
		network: 'ethereum-mainnet',
		scope: { ...defaultGraphOptions },
		createdAt: now,
		updatedAt: now,
		rootAddress: '',
		notes: '',
		selectedAddressIds: [],
		selectedTransferIds: [],
		annotations: [],
	};
}

export function loadLocalCase(): LocalCase {
	if (typeof window === 'undefined') return createLocalCase();
	const stored = window.localStorage.getItem(STORAGE_KEY);
	if (!stored) return createLocalCase();
	try {
		return parseCaseFile(stored);
	} catch {
		return createLocalCase();
	}
}

export function saveLocalCase(caseFile: LocalCase): void {
	if (typeof window !== 'undefined')
		window.localStorage.setItem(STORAGE_KEY, JSON.stringify(caseFile));
}

export function parseCaseFile(value: string): LocalCase {
	const parsed: unknown = JSON.parse(value);
	if (!parsed || typeof parsed !== 'object') throw new Error('Case file must be an object.');
	const item = parsed as Partial<LocalCase>;
	const strings = (items: unknown): items is string[] =>
		Array.isArray(items) && items.every((entry) => typeof entry === 'string');
	const annotations =
		Array.isArray(item.annotations) &&
		item.annotations.every((entry) => {
			if (!entry || typeof entry !== 'object') return false;
			const annotation = entry as Partial<CaseAnnotation>;
			return (
				typeof annotation.id === 'string' &&
				typeof annotation.note === 'string' &&
				typeof annotation.createdAt === 'string' &&
				!!annotation.target &&
				typeof annotation.target.id === 'string' &&
				(annotation.target.kind === 'address' || annotation.target.kind === 'transfer')
			);
		});
	const scope = item.scope as Partial<GraphOptions> | undefined;
	const validScope =
		!!scope &&
		[5, 10, 25].includes(scope.maxCounterparties ?? 0) &&
		[1, 2, 3, 4].includes(scope.ranking ?? 0) &&
		[1, 2, 3].includes(scope.direction ?? 0) &&
		[1, 2, 3].includes(scope.maxDepth ?? 0) &&
		typeof scope.from === 'string' &&
		typeof scope.to === 'string' &&
		typeof scope.asset === 'string' &&
		typeof scope.minimumAmount === 'string' &&
		typeof scope.transferKind === 'string';
	if (
		item.version !== CASE_FILE_VERSION ||
		!isNetworkSlug(item.network) ||
		typeof item.title !== 'string' ||
		typeof item.notes !== 'string' ||
		typeof item.rootAddress !== 'string' ||
		typeof item.createdAt !== 'string' ||
		typeof item.updatedAt !== 'string' ||
		!strings(item.selectedAddressIds) ||
		!strings(item.selectedTransferIds) ||
		!annotations ||
		!validScope
	)
		throw new Error('Unsupported case file.');
	return { ...item, scope: { ...scope } } as LocalCase;
}
