// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { Network } from '../services/api';
import { Header } from './Header';

describe('Header Component', () => {
	it('renders brand title and network name', () => {
		const onNetworkChange = vi.fn();
		render(
			<Header
				currentAddress="0x1234"
				onSearch={vi.fn()}
				network={Network.BASE_MAINNET}
				onNetworkChange={onNetworkChange}
			/>,
		);
		expect(screen.getByText('OpenChain')).toBeTruthy();
		expect(screen.getByAltText('Base Mainnet icon')).toBeTruthy();
		fireEvent.click(screen.getByLabelText('Network'));
		expect(screen.getByRole('button', { name: 'Base Mainnet' })).toBeTruthy();
		expect(screen.getByRole('button', { name: 'Solana Mainnet' })).toBeTruthy();
		expect(screen.getByRole('button', { name: 'TRON Mainnet' })).toBeTruthy();
		fireEvent.change(screen.getByPlaceholderText('Search target address'), {
			target: { value: '0x0000000000000000000000000000000000000000' },
		});
		expect(onNetworkChange).not.toHaveBeenCalled();
	});
});
