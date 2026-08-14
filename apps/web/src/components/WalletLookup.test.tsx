// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { AddressLabel, type AddressSummary, Network } from '../services/api';
import { WalletLookup, entityCategoryName } from './WalletLookup';

describe('WalletLookup entity evidence', () => {
	it('keeps verified entity evidence distinct and shows its source details', () => {
		render(
			<WalletLookup
				summary={{ address: '0x1', balanceFormatted: '0 ETH' } as AddressSummary}
				labels={[
					new AddressLabel({
						id: 'merchant',
						category: 'otc',
						label: 'Cash desk',
						confidence: 0.85,
						source: 'Public registry',
						sourceVersion: '2026-08',
						evidenceUrl: 'https://example.test/proof',
					}),
				]}
				fieldStatuses={[]}
				loading={false}
				onTraceAddress={() => undefined}
				network={Network.ETHEREUM_MAINNET}
			/>,
		);
		expect(screen.getByText('Verified entity labels')).toBeTruthy();
		expect(screen.getByText('OTC merchant: Cash desk')).toBeTruthy();
		expect(screen.getByText(/85% confidence/)).toBeTruthy();
		expect(screen.getByText(/Source: Public registry · 2026-08/)).toBeTruthy();
		expect(screen.getByRole('link', { name: 'Proof' }).getAttribute('href')).toBe(
			'https://example.test/proof',
		);
	});

	it('names canonical entity categories for investigators', () => {
		expect(entityCategoryName('sanctioned-service')).toBe('Sanctioned service');
		expect(entityCategoryName('high-risk-service')).toBe('High-risk service');
	});
});
