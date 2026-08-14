import { createHash } from 'node:crypto';
import { expect, test } from '@playwright/test';

test.beforeEach(async ({ page }) => {
	await page.goto('/');
	await expect(page.getByPlaceholder('Search target address')).toBeVisible();
	await page.getByRole('tab', { name: 'Case' }).click();
	await expect(page.getByLabel('Case title')).toBeVisible();
});

async function selectNetwork(page: import('@playwright/test').Page, name: string) {
	await page.getByLabel('Network').click();
	await page.getByRole('button', { name }).click();
}

function connectJSON(message: unknown) {
	return {
		body: JSON.stringify(message),
		headers: { 'content-type': 'application/json', 'connect-protocol-version': '1' },
	};
}

function frozenEvidencePackage(root: string, transferID: string, includeBridge = false) {
	const bridge = includeBridge
		? {
			id: 'base-standard-bridge:test-message', protocol: 'base-standard-bridge', bridge_name: 'Base Standard Bridge', source_network: 'ethereum-mainnet', destination_network: 'base-mainnet', lifecycle: 'finalized', message_id: '0xmessage', source_transfer_id: transferID, destination_transfer_id: '', source_transaction_hash: '0xtx', destination_transaction_hash: '0xdestination', source_log_reference: '0xtx:log:1', destination_log_reference: '0xdestination:log:2', source_bridge_address: '0x3154cf16ccdb4c6d922629664174b904d80f2c35', destination_bridge_address: '0x4200000000000000000000000000000000000010', canonical_source_token: '', canonical_destination_token: '', recipient: '0x0000000000000000000000000000000000000003', asset: { kind: 'NATIVE', contract_address: '', symbol: 'ETH', decimals: 18 }, amount_base_units: '42', source_block_number: 1, destination_block_number: 2, source_block_hash: '0xblock', destination_block_hash: '0xdestinationblock', source_timestamp: '2026-08-11T00:00:00Z', destination_timestamp: '2026-08-11T00:01:00Z', source_confirmed: true, destination_confirmed: true, limitations: 'No ownership or intent inference.',
		}
		: null;
	const payload = {
		exported_at: '2026-08-12T00:00:00Z',
		case: {
			version: 3,
			title: 'Deterministic replay',
			network: 'ethereum-mainnet',
			scope: {
				maxCounterparties: 10,
				ranking: 1,
				direction: 1,
				maxDepth: 1,
				from: '',
				to: '',
				asset: '',
				minimumAmount: '',
				transferKind: '',
			},
			createdAt: '2026-08-12T00:00:00Z',
			updatedAt: '2026-08-12T00:00:00Z',
			rootAddress: root,
			notes: 'No live provider is used.',
			selectedAddressIds: [],
			selectedTransferIds: [transferID],
			pinnedBridgeTransitionIds: bridge ? [bridge.id] : [],
			annotations: [],
		},
		transfers: [{ id: transferID, network: 'ethereum-mainnet', transaction_hash: '0xtx', event_id: 'tx', transfer_kind: 'NATIVE', from_address: root, to_address: '0x0000000000000000000000000000000000000002', asset: { kind: 'NATIVE', contract_address: '', symbol: 'ETH', decimals: 18 }, amount_base_units: '42', block_number: 1, block_hash: '0xblock', block_timestamp: '2026-08-11T00:00:00Z', provisional: false, source: 'frozen-test', retrieved_at: '2026-08-12T00:00:00Z' }],
		acquisition_snapshots: bridge ? [{ id: 1, network: 'ethereum-mainnet', provider: 'fixture', request_identity: 'fixture', response_sha256: 'fixture', response_body_base64: '', retrieved_at: '2026-08-12T00:00:00Z' }] : [],
		acquisition_scopes: [],
		scope_transfers: [],
		scope_snapshots: [],
		rule_runs: [],
		labels: [],
		bridge_transitions: bridge ? [bridge] : [],
		bridge_transition_acquisitions: bridge ? [{ transition_id: bridge.id, side: 'source', acquisition_id: 1 }] : [],
	};
	return JSON.stringify({
		format: 'openchain-evidence-package',
		version: 3,
		payload,
		manifest: { algorithm: 'SHA-256', payload_sha256: createHash('sha256').update(JSON.stringify(payload)).digest('hex') },
	});
}

test('preserves an explicit EVM choice and detects address families', async ({ page }) => {
	const network = page.getByLabel('Network');
	const address = page.getByPlaceholder('Search target address');

	await selectNetwork(page, 'Base Mainnet');
	await expect(network).toContainText('Base Mainnet');
	await address.fill('0x7a250d5630b4cf539739df2c5dacb4c659f2488d');
	await expect(network).toContainText('Base Mainnet');
	await selectNetwork(page, 'Polygon Mainnet');
	await expect(network).toContainText('Polygon Mainnet');
	await address.fill('0x7a250d5630b4cf539739df2c5dacb4c659f2488d');
	await expect(network).toContainText('Polygon Mainnet');

	await selectNetwork(page, 'Solana Mainnet');
	await address.fill('');
	await address.fill('0x7a250d5630b4cf539739df2c5dacb4c659f2488d');
	await expect(network).toContainText('Ethereum Mainnet');

	await address.fill('11111111111111111111111111111111');
	await expect(network).toContainText('Solana Mainnet');

	await address.fill('TXLAQ63Xg1NAzckPwKHvzw7CSEmLMEqcdj');
	await expect(network).toContainText('TRON Mainnet');
});

test('persists local investigation notes across a reload', async ({ page }) => {
	await page.getByLabel('Case title').fill('E2E investigation');
	await page.getByLabel('Case notes').fill('Verified in Chromium.');
	await expect
		.poll(() =>
			page.evaluate(() => JSON.parse(localStorage.getItem('openchain.local-case.v3') || '{}').title),
		)
		.toBe('E2E investigation');
	await page.reload();
	await page.waitForTimeout(150);
	await page.getByRole('tab', { name: 'Case' }).click();
	await expect(page.getByRole('tab', { name: 'Case' })).toHaveAttribute('aria-selected', 'true');
	await expect(page.getByLabel('Case title')).toHaveValue('E2E investigation');
	await expect(page.getByLabel('Case notes')).toHaveValue('Verified in Chromium.');
});

test('provides focused graph filters without leaving the investigation workspace', async ({ page }) => {
	await page.getByRole('button', { name: 'Filters' }).click();
	await expect(page.getByLabel('Counterparties per address')).toHaveValue('10');
	await page.getByLabel('Counterparties per address').selectOption('5');
	await expect(page.getByLabel('Counterparties per address')).toHaveValue('5');
	await expect(page.getByLabel('Investigation direction')).toHaveValue('1');
	await page.getByLabel('Investigation direction').selectOption('2');
	await expect(page.getByLabel('Investigation direction')).toHaveValue('2');
	await expect(page.getByLabel('Maximum graph depth')).toHaveValue('1');
	await page.getByLabel('Maximum graph depth').selectOption('2');
	await expect(page.getByLabel('Maximum graph depth')).toHaveValue('2');
	await expect(page.getByLabel('Counterparty ranking')).toHaveValue('1');
	await page.getByLabel('Counterparty ranking').selectOption('4');
	await expect(page.getByLabel('Counterparty ranking')).toHaveValue('4');
	await expect(page.getByLabel('From date')).toBeVisible();
	await expect(page.getByLabel('To date')).toBeVisible();
	await expect(page.getByLabel('Asset')).toBeVisible();
	await expect(page.getByLabel('Minimum amount')).toBeVisible();
	await expect(page.getByLabel('Transfer type')).toBeVisible();
});

test('restores a shareable investigation scope from the URL', async ({ page }) => {
	const root = '0x0000000000000000000000000000000000000001';
	await page.route('**/openchain.v1.TracingService/TraceGraph', (route) =>
		route.fulfill(
			connectJSON({
				seedAddress: root,
				nodes: [{ id: root, label: 'Shared target', isSeed: true }],
				sourceStatus: { source: 'test-fixture', isComplete: true },
			}),
		),
	);
	await page.route('**/openchain.v1.LookupService/LookupAddress', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.route('**/openchain.v1.LabelService/GetLabels', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.goto(
		`/?network=base-mainnet&address=${root}&counterparties=5&ranking=2&direction=3&depth=2`,
	);
	await expect(page.getByLabel('Network')).toContainText('Base Mainnet');
	await expect(page.getByPlaceholder('Search target address')).toHaveValue(root);
	await page.getByRole('button', { name: 'Filters' }).click();
	await expect(page.getByLabel('Counterparties per address')).toHaveValue('5');
	await expect(page.getByLabel('Counterparty ranking')).toHaveValue('2');
	await expect(page.getByLabel('Investigation direction')).toHaveValue('3');
	await expect(page.getByLabel('Maximum graph depth')).toHaveValue('2');
});

test('guides source and destination tracing from the target address', async ({ page }) => {
	const root = '0x0000000000000000000000000000000000000001';
	const directions: string[] = [];
	await page.route('**/openchain.v1.TracingService/TraceGraph', (route) => {
		directions.push(JSON.parse(route.request().postData() || '{}').direction);
		return route.fulfill(
			connectJSON({
				seedAddress: root,
				nodes: [{ id: root, label: 'Test seed', isSeed: true }],
				sourceStatus: { source: 'test-fixture', isComplete: true },
			}),
		);
	});
	await page.route('**/openchain.v1.LookupService/LookupAddress', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.route('**/openchain.v1.LabelService/GetLabels', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.getByPlaceholder('Search target address').fill(root);
	await page.getByLabel('Investigate address').click();
	await expect(page.getByRole('button', { name: 'Trace source of funds' })).toBeVisible();
	await page.getByRole('button', { name: 'Trace source of funds' }).click();
	await expect.poll(() => directions.at(-1)).toBe('TRACE_DIRECTION_INBOUND');
	await page.getByRole('button', { name: 'Trace destination of funds' }).click();
	await expect.poll(() => directions.at(-1)).toBe('TRACE_DIRECTION_OUTBOUND');
});

test('replays a deterministic queued trace through finding, package export, and import', async ({ page }) => {
	const root = '0x0000000000000000000000000000000000000001';
	const target = '0x0000000000000000000000000000000000000002';
	const transferID = 'ethereum-mainnet:mock-tx:tx';
	const pending = {
		seedAddress: root,
		nodes: [{ id: root, label: 'Test seed', isSeed: true }],
		pending: true,
	};
	const completed = {
		seedAddress: root,
		nodes: [
			{ id: root, label: 'Test seed', isSeed: true },
			{ id: target, label: 'Counterparty' },
		],
		edges: [{ id: transferID, source: root, target, amountBaseUnits: '42', txCount: 1, asset: { kind: 'NATIVE', symbol: 'ETH', decimals: 18 }, firstTxTimestamp: '1', lastTxTimestamp: '1', eventId: 'tx', blockNumber: '1', transactionHash: '0xtx', transferKind: 'NATIVE', sourceName: 'test-fixture', retrievedAt: '1' }],
		leads: [{ id: 'fan-out-dispersion:test', ruleId: 'fan-out-dispersion', ruleVersion: '1.0.0', title: 'Fan-out dispersion lead', subjectAddress: root, transferIds: [transferID], rationale: 'Observed deterministic distribution.', limitations: 'Requires human review.', parametersJson: '{"window_seconds":3600,"min_distinct_counterparties":3}' }],
		totalNodes: 2,
		totalEdges: 1,
		hasMore: true,
		sourceStatus: { source: 'test-fixture', isComplete: true },
	};
	const packageJSON = frozenEvidencePackage(root, transferID);
	let statusRequests = 0;
	let expansionRequests = 0;
	const traceDirections: string[] = [];
	let exportRequests = 0;
	let exportedDepth = 0;
	await page.route('**/openchain.v1.TracingService/TraceGraph', (route) => {
		traceDirections.push(JSON.parse(route.request().postData() || '{}').direction);
		return route.fulfill(connectJSON(pending));
	});
	await page.route('**/openchain.v1.TracingService/GetTraceStatus', (route) => {
		statusRequests++;
		return route.fulfill(connectJSON(completed));
	});
	await page.route('**/openchain.v1.TracingService/ExpandNode', (route) => {
		expansionRequests++;
		return route.fulfill(
			connectJSON({
				newNodes: [{ id: '0x0000000000000000000000000000000000000003', label: 'Expanded source' }],
				newEdges: [{ id: 'ethereum-mainnet:expanded:tx', source: '0x0000000000000000000000000000000000000003', target: root, amountBaseUnits: '41', txCount: 1, asset: { kind: 'NATIVE', symbol: 'ETH', decimals: 18 }, firstTxTimestamp: '2', lastTxTimestamp: '2', eventId: 'tx', blockNumber: '2', transactionHash: '0xexpanded', transferKind: 'NATIVE', sourceName: 'test-fixture', retrievedAt: '2' }],
				hasMore: false,
			}),
		);
	});
	await page.route('**/openchain.v1.LookupService/LookupAddress', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.route('**/openchain.v1.LabelService/GetLabels', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.route('**/openchain.v1.EvidenceService/ExportEvidencePackage', (route) => {
		exportRequests++;
		const request = JSON.parse(route.request().postData() || '{}');
		exportedDepth = JSON.parse(request.caseJson).scope.maxDepth;
		return route.fulfill(connectJSON({ packageJson: packageJSON }));
	});
	await page.getByRole('button', { name: 'Filters' }).click();
	await page.getByLabel('Maximum graph depth').selectOption('2');
	await expect(page.getByLabel('Maximum graph depth')).toHaveValue('2');
	await expect
		.poll(() =>
			page.evaluate(
				() => JSON.parse(localStorage.getItem('openchain.local-case.v3') || '{}').scope?.maxDepth,
			),
		)
		.toBe(2);
	await page.getByPlaceholder('Search target address').fill(root);
	await page.getByLabel('Investigate address').click();
	await expect(page.getByText('Retrieving address flow…')).toBeVisible();
	await page.getByRole('tab', { name: /Findings/ }).click();
	await expect(page.getByText('Fan-out dispersion lead')).toBeVisible({ timeout: 10_000 });
	expect(await page.locator('canvas').first().evaluate((canvas) => canvas.getBoundingClientRect().height)).toBeGreaterThan(200);
	expect(statusRequests).toBe(1);
	await page.getByRole('button', { name: 'Trace source of funds' }).click();
	await expect.poll(() => statusRequests, { timeout: 10_000 }).toBe(2);
	expect(traceDirections.at(-1)).toBe('TRACE_DIRECTION_INBOUND');
	await page.getByRole('button', { name: 'Expand' }).click();
	await expect.poll(() => expansionRequests).toBe(1);
	await expect(page.getByRole('button', { name: 'Expand' })).toBeDisabled();
	await page.getByRole('tab', { name: 'Case' }).click();
	const download = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Package' }).click();
	expect((await download).suggestedFilename()).toBe('openchain-evidence-package.json');
	expect(exportRequests).toBe(1);
	expect(exportedDepth).toBe(2);
	await page.locator('input[type=file]').setInputFiles({ name: 'evidence.json', mimeType: 'application/json', buffer: Buffer.from(packageJSON) });
	await expect(page.getByLabel('Case title')).toHaveValue('Deterministic replay');
	await expect(page.getByRole('button', { name: 'Print' })).toBeEnabled();
});

test('preserves Base58 address casing while polling an expansion', async ({ page }) => {
	const root = 'TXLAQ63Xg1NAzckPwKHvzw7CSEmLMEqcdj';
	let polledAddress = '';
	await page.route('**/openchain.v1.TracingService/TraceGraph', (route) =>
		route.fulfill(
			connectJSON({
				seedAddress: root,
				nodes: [{ id: root, label: 'TRON seed', isSeed: true }],
				hasMore: true,
				sourceStatus: { source: 'test-fixture', isComplete: true },
			}),
		),
	);
	await page.route('**/openchain.v1.TracingService/ExpandNode', (route) =>
		route.fulfill(connectJSON({ pending: true })),
	);
	await page.route('**/openchain.v1.TracingService/GetTraceStatus', async (route) => {
		polledAddress = JSON.parse(route.request().postData() || '{}').address;
		await route.fulfill(
			connectJSON({
				seedAddress: root,
				nodes: [{ id: root, label: 'TRON seed', isSeed: true }],
				sourceStatus: { source: 'test-fixture', isComplete: true },
			}),
		);
	});
	await page.route('**/openchain.v1.LookupService/LookupAddress', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.route('**/openchain.v1.LabelService/GetLabels', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.goto('/');
	await selectNetwork(page, 'TRON Mainnet');
	await page.getByPlaceholder('Search target address').fill(root);
	await page.getByLabel('Investigate address').click();
	await page.getByRole('button', { name: 'Expand' }).click();
	await expect.poll(() => polledAddress, { timeout: 10_000 }).toBe(root);
});

test('verifies and replays an imported frozen evidence package locally', async ({ page }) => {
	const root = '0x0000000000000000000000000000000000000001';
	const packageJSON = frozenEvidencePackage(root, 'ethereum-mainnet:tx:tx');
	await page.locator('input[type=file]').setInputFiles({ name: 'evidence.json', mimeType: 'application/json', buffer: Buffer.from(packageJSON) });
	await expect(page.getByLabel('Case title')).toHaveValue('Deterministic replay');
	await expect(page.getByRole('button', { name: 'Print' })).toBeEnabled();
});

test('follows a verified Base bridge path, traces its destination, and preserves it in replay', async ({ page }) => {
	const root = '0x0000000000000000000000000000000000000001';
	const recipient = '0x0000000000000000000000000000000000000003';
	const transferID = 'ethereum-mainnet:bridge:tx';
	const bridge = {
		id: 'base-standard-bridge:test-message', protocol: 'base-standard-bridge', bridgeName: 'Base Standard Bridge', sourceNetwork: 'NETWORK_ETHEREUM_MAINNET', destinationNetwork: 'NETWORK_BASE_MAINNET', lifecycle: 'BRIDGE_LIFECYCLE_FINALIZED', messageId: '0xmessage', sourceTransferId: transferID, sourceTransactionHash: '0xtx', destinationTransactionHash: '0xdestination', sourceLogReference: '0xtx:log:1', destinationLogReference: '0xdestination:log:2', sourceBridgeAddress: '0x3154cf16ccdb4c6d922629664174b904d80f2c35', destinationBridgeAddress: '0x4200000000000000000000000000000000000010', recipient, asset: { kind: 'NATIVE', symbol: 'ETH', decimals: 18 }, amountBaseUnits: '42', sourceBlockNumber: '1', destinationBlockNumber: '2', sourceTimestamp: '1', destinationTimestamp: '2', sourceConfirmed: true, destinationConfirmed: true, limitations: 'No ownership or intent inference.',
	};
	let requests = 0;
	await page.route('**/openchain.v1.TracingService/TraceGraph', (route) => {
		requests++;
		const body = JSON.parse(route.request().postData() || '{}');
		const seed = body.address === recipient ? recipient : root;
		return route.fulfill(connectJSON({
			seedAddress: seed,
			nodes: seed === root
				? [{ id: root, label: 'Bridge test seed', isSeed: true }, { id: '0x3154cf16ccdb4c6d922629664174b904d80f2c35', label: 'Base bridge' }]
				: [{ id: seed, label: 'Bridge test seed', isSeed: true }],
			edges: seed === root ? [{ id: transferID, source: root, target: '0x3154cf16ccdb4c6d922629664174b904d80f2c35', amountBaseUnits: '42', txCount: 1, asset: { kind: 'NATIVE', symbol: 'ETH', decimals: 18 }, eventId: 'tx', blockNumber: '1', transactionHash: '0xtx', transferKind: 'NATIVE', sourceName: 'fixture', retrievedAt: '1', firstTxTimestamp: '1', lastTxTimestamp: '1' }] : [],
			crossChainTransitions: seed === root ? [bridge] : [],
			sourceStatus: { source: 'fixture', isComplete: true },
		}));
	});
	await page.route('**/openchain.v1.LookupService/LookupAddress', (route) => route.fulfill(connectJSON({})));
	await page.route('**/openchain.v1.LabelService/GetLabels', (route) => route.fulfill(connectJSON({})));
	await page.route('**/openchain.v1.EvidenceService/ExportEvidencePackage', (route) =>
		route.fulfill(connectJSON({ packageJson: frozenEvidencePackage(root, transferID, true) })),
	);
	await page.getByPlaceholder('Search target address').fill(root);
	await page.getByLabel('Investigate address').click();
	await page.getByRole('tab', { name: /Evidence/ }).click();
	await expect(page.getByText('Base Standard Bridge')).toBeVisible();
	await page.getByRole('tab', { name: 'Case' }).click();
	const download = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Package' }).click();
	await download;
	await page.getByRole('tab', { name: /Evidence/ }).click();
	await page.getByRole('button', { name: 'Trace destination recipient' }).click();
	await expect.poll(() => requests).toBe(2);
	await expect(page.getByLabel('Network')).toContainText('Base Mainnet');
	await expect(page.getByPlaceholder('Search target address')).toHaveValue(recipient);
	await page.getByRole('tab', { name: 'Case' }).click();
	await page.locator('input[type=file]').setInputFiles({ name: 'bridge-evidence.json', mimeType: 'application/json', buffer: Buffer.from(frozenEvidencePackage(root, transferID, true)) });
	await page.getByRole('tab', { name: /Evidence/ }).click();
	await expect(page.getByText('Base Standard Bridge')).toBeVisible();
});
