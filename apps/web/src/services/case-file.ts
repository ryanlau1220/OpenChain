export const CASE_FILE_VERSION = 1;
const STORAGE_KEY = 'openchain.local-case.v1';

export type CaseAnnotation = {
	id: string;
	target: { kind: 'address' | 'transfer'; id: string };
	note: string;
	createdAt: string;
};

export type LocalCase = {
	version: number;
	title: string;
	network: 'ethereum-mainnet' | 'base-mainnet' | 'solana-mainnet' | 'tron-mainnet';
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
	if (
		item.version !== CASE_FILE_VERSION ||
		(item.network !== 'ethereum-mainnet' &&
			item.network !== 'base-mainnet' &&
			item.network !== 'solana-mainnet' &&
			item.network !== 'tron-mainnet') ||
		typeof item.title !== 'string' ||
		typeof item.notes !== 'string' ||
		typeof item.rootAddress !== 'string' ||
		typeof item.createdAt !== 'string' ||
		typeof item.updatedAt !== 'string' ||
		!strings(item.selectedAddressIds) ||
		!strings(item.selectedTransferIds) ||
		!annotations
	)
		throw new Error('Unsupported case file.');
	return item as LocalCase;
}
