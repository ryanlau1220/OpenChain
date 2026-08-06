// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { Header } from './Header';

describe('Header Component', () => {
	it('renders brand title and network name', () => {
		render(
			<Header
				currentAddress="0x1234"
				onSearch={vi.fn()}
				network="Sepolia Testnet"
			/>,
		);
		expect(screen.getByText('OpenChain')).toBeTruthy();
		expect(screen.getByText('Sepolia Testnet')).toBeTruthy();
	});
});
