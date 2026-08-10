import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { Network, explorerURL, requestErrorMessage } from './api';

describe('requestErrorMessage', () => {
	it('shows a retryable quota message', () => {
		expect(
			requestErrorMessage(
				new ConnectError('request limit reached; try again in one minute', Code.ResourceExhausted),
				'fallback',
			),
		).toBe('request limit reached; try again in one minute');
	});

	it('uses the matching explorer for every supported network', () => {
		expect(explorerURL(Network.BASE_MAINNET, 'tx', '0xabc')).toBe('https://basescan.org/tx/0xabc');
		expect(explorerURL(Network.SOLANA_MAINNET, 'tx', 'signature')).toBe(
			'https://explorer.solana.com/tx/signature',
		);
		expect(explorerURL(Network.TRON_MAINNET, 'address', 'TAddress')).toBe(
			'https://tronscan.org/#/address/TAddress',
		);
	});
});
