import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { requestErrorMessage } from './api';

describe('requestErrorMessage', () => {
	it('shows a retryable quota message', () => {
		expect(requestErrorMessage(new ConnectError('request limit reached; try again in one minute', Code.ResourceExhausted), 'fallback')).toBe('request limit reached; try again in one minute');
	});
});
