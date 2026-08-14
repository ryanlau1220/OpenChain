import { describe, expect, it } from 'vitest';
import { formatObservationTime } from './format';

describe('formatObservationTime', () => {
	it('renders a UTC observation timestamp and rejects invalid input', () => {
		expect(formatObservationTime(0n)).toContain('1970');
		expect(formatObservationTime(999999999999999999999999999999n)).toBe('Unknown observation time');
	});
});
