import { describe, expect, it, vi } from 'vitest';
import { getExportUrl } from './api';

describe('API Service Helper', () => {
	it('generates export URL correctly', () => {
		const urlJSON = getExportUrl('CASE-001', 'JSON');
		expect(urlJSON).toContain(
			'/api/v1/cases/export?case_id=CASE-001&format=JSON',
		);

		const urlCSV = getExportUrl('CASE-002', 'CSV');
		expect(urlCSV).toContain(
			'/api/v1/cases/export?case_id=CASE-002&format=CSV',
		);
	});
});
