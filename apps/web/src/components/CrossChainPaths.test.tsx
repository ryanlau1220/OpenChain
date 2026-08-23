// @vitest-environment jsdom
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { BridgeLifecycle, CrossChainTransition, Network } from '../services/api';
import { CrossChainPaths } from './CrossChainPaths';

describe('CrossChainPaths', () => {
	it('renders qualified bridge evidence without asserting wallet ownership', () => {
		render(
			<CrossChainPaths
				transitions={[
					new CrossChainTransition({
						id: 'cross-chain:source:destination',
						bridgeName: 'Base Standard Bridge',
						sourceNetwork: Network.ETHEREUM_MAINNET,
						destinationNetwork: Network.BASE_MAINNET,
						sourceTransactionHash: '0xsource',
						destinationTransactionHash: '0xdestination',
						amountBaseUnits: '1000000',
						sourceTimestamp: 100n,
						destinationTimestamp: 200n,
						sourceBridgeAddress: '0xsourcebridge',
						destinationBridgeAddress: '0xdestinationbridge',
						limitations: 'This does not establish cross-chain address ownership.',
					}),
				]}
			/>,
		);
		expect(screen.getByText('Cross-chain continuations')).toBeTruthy();
		expect(screen.getByText('Base Standard Bridge')).toBeTruthy();
		expect(screen.getAllByText(/does not establish cross-chain address ownership/i)).toHaveLength(
			2,
		);
		expect(screen.getByRole('link', { name: /source transaction/i }).getAttribute('href')).toBe(
			'https://etherscan.io/tx/0xsource',
		);
		expect(
			screen.getByRole('link', { name: /destination transaction/i }).getAttribute('href'),
		).toBe('https://basescan.org/tx/0xdestination');
	});

	it('traces a finalized Optimism bridge recipient on its destination network', () => {
		const onTraceDestination = vi.fn();
		render(
			<CrossChainPaths
				transitions={[
					new CrossChainTransition({
						id: 'optimism:source:destination',
						bridgeName: 'Optimism Standard Bridge',
						sourceNetwork: Network.ETHEREUM_MAINNET,
						destinationNetwork: Network.OPTIMISM_MAINNET,
						sourceTransactionHash: '0xsource',
						destinationTransactionHash: '0xdestination',
						recipient: '0xrecipient',
						lifecycle: BridgeLifecycle.FINALIZED,
					}),
				]}
				onTraceDestination={onTraceDestination}
			/>,
		);
		expect(
			screen.getByRole('link', { name: /destination transaction/i }).getAttribute('href'),
		).toBe('https://optimistic.etherscan.io/tx/0xdestination');
		screen.getByRole('button', { name: /trace destination recipient/i }).click();
		expect(onTraceDestination).toHaveBeenCalledWith(
			expect.objectContaining({ destinationNetwork: Network.OPTIMISM_MAINNET }),
		);
	});
});
