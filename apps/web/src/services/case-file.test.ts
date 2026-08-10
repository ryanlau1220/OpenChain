import { describe, expect, it } from 'vitest';
import { CASE_FILE_VERSION, caseCSV, createLocalCase, loadLocalCase, parseCaseFile, saveLocalCase } from './case-file';

describe('local case files', () => {
	it('round-trips a versioned case and escapes CSV notes', () => {
		const caseFile = createLocalCase();
		caseFile.annotations.push({ id: 'a1', target: { kind: 'address', id: '0x1' }, note: 'note, with "evidence"', createdAt: caseFile.createdAt });
		const parsed = parseCaseFile(JSON.stringify(caseFile));
		expect(parsed.version).toBe(CASE_FILE_VERSION);
		expect(caseCSV(parsed)).toContain('"note, with ""evidence"""');
	});

	it('rejects malformed imports and restores a saved case', () => {
		expect(() => parseCaseFile('{"version":1,"network":"ethereum-mainnet","annotations":[{}]}')).toThrow();
		const caseFile = createLocalCase();
		caseFile.title = 'Saved locally';
		saveLocalCase(caseFile);
		expect(loadLocalCase().title).toBe('Saved locally');
	});
});
