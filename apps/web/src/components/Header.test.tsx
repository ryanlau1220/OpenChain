// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { Network } from '../services/api';
import { Header } from './Header';

describe('Header Component', () => {
	it('renders brand title and network name', () => {
		render(
			<Header
				currentAddress="0x1234"
				onSearch={vi.fn()}
				network={Network.BASE_MAINNET}
				onNetworkChange={vi.fn()}
			/>,
		);
		expect(screen.getByText('OpenChain')).toBeTruthy();
		expect(screen.getByRole('option', { name: 'Base Mainnet' })).toBeTruthy();
	});
});
