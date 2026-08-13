import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { Network, detectAddressNetwork, explorerURL, requestErrorMessage } from './api';

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
		expect(explorerURL(Network.BNB_CHAIN, 'address', '0xabc')).toBe(
			'https://bscscan.com/address/0xabc',
		);
	});

	it('detects address families and defaults ambiguous EVM addresses to Ethereum', () => {
		expect(detectAddressNetwork('11111111111111111111111111111111')).toBe(Network.SOLANA_MAINNET);
		expect(detectAddressNetwork('T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb')).toBe(Network.TRON_MAINNET);
		expect(detectAddressNetwork('0x0000000000000000000000000000000000000000')).toBe(
			Network.ETHEREUM_MAINNET,
		);
		expect(detectAddressNetwork('EQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAM9c')).toBe(
			Network.TON_MAINNET,
		);
		expect(
			detectAddressNetwork(
				'addr1qabcdefghijklmnopqrstuvxyz023456789abcdefghijklmnopqrstuvxyz023456789',
			),
		).toBe(Network.CARDANO_MAINNET);
	});
});
