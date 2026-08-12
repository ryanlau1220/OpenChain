import { beforeEach, describe, expect, it } from 'vitest';
import {
	CASE_FILE_VERSION,
	createLocalCase,
	loadLocalCase,
	parseCaseFile,
	saveLocalCase,
} from './case-file';

describe('local case files', () => {
	beforeEach(() => {
		const values = new Map<string, string>();
		Object.defineProperty(window, 'localStorage', {
			configurable: true,
			value: {
				getItem: (key: string) => values.get(key) ?? null,
				setItem: (key: string, value: string) => values.set(key, value),
			},
		});
	});

	it('round-trips a versioned local case', () => {
		const caseFile = createLocalCase();
		const parsed = parseCaseFile(JSON.stringify(caseFile));
		expect(parsed.version).toBe(CASE_FILE_VERSION);
	});

	it('rejects malformed imports and restores a saved case', () => {
		expect(() =>
			parseCaseFile('{"version":1,"network":"ethereum-mainnet","annotations":[{}]}'),
		).toThrow();
		const caseFile = createLocalCase();
		caseFile.title = 'Saved locally';
		saveLocalCase(caseFile);
		expect(loadLocalCase().title).toBe('Saved locally');
	});

	it('keeps Base case files separate from Ethereum case files', () => {
		const caseFile = createLocalCase();
		caseFile.network = 'solana-mainnet';
		expect(parseCaseFile(JSON.stringify(caseFile)).network).toBe('solana-mainnet');
	});
});
