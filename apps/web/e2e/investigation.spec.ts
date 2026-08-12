import { createHash } from 'node:crypto';
import { expect, test } from '@playwright/test';

test.beforeEach(async ({ page }) => {
	await page.goto('/');
	await expect(page.getByPlaceholder('Search target address')).toBeVisible();
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

function frozenEvidencePackage(root: string, transferID: string) {
	const payload = {
		exported_at: '2026-08-12T00:00:00Z',
		case: {
			version: 1,
			title: 'Deterministic replay',
			network: 'ethereum-mainnet',
			createdAt: '2026-08-12T00:00:00Z',
			updatedAt: '2026-08-12T00:00:00Z',
			rootAddress: root,
			notes: 'No live provider is used.',
			selectedAddressIds: [],
			selectedTransferIds: [transferID],
			annotations: [],
		},
		transfers: [{ id: transferID, network: 'ethereum-mainnet', transaction_hash: '0xtx', event_id: 'tx', transfer_kind: 'NATIVE', from_address: root, to_address: '0x0000000000000000000000000000000000000002', asset: { kind: 'NATIVE', contract_address: '', symbol: 'ETH', decimals: 18 }, amount_base_units: '42', block_number: 1, block_hash: '0xblock', block_timestamp: '2026-08-11T00:00:00Z', provisional: false, source: 'frozen-test', retrieved_at: '2026-08-12T00:00:00Z' }],
		acquisition_snapshots: [],
		provenance: [],
		rule_runs: [],
		labels: [],
	};
	return JSON.stringify({
		format: 'openchain-evidence-package',
		version: 1,
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
			page.evaluate(() => JSON.parse(localStorage.getItem('openchain.local-case.v1') || '{}').title),
		)
		.toBe('E2E investigation');
	await page.reload();

	await expect(page.getByLabel('Case title')).toHaveValue('E2E investigation');
	await expect(page.getByLabel('Case notes')).toHaveValue('Verified in Chromium.');
});

test('provides focused graph filters without leaving the investigation workspace', async ({ page }) => {
	await page.getByText('Filters', { exact: true }).click();
	await expect(page.getByLabel('From date')).toBeVisible();
	await expect(page.getByLabel('To date')).toBeVisible();
	await expect(page.getByLabel('Transfer direction')).toBeVisible();
	await expect(page.getByLabel('Asset')).toBeVisible();
	await expect(page.getByLabel('Minimum amount')).toBeVisible();
	await expect(page.getByLabel('Transfer type')).toBeVisible();
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
		sourceStatus: { source: 'test-fixture', isComplete: true },
	};
	const packageJSON = frozenEvidencePackage(root, transferID);
	let statusRequests = 0;
	let exportRequests = 0;
	await page.route('**/openchain.v1.TracingService/TraceGraph', (route) =>
		route.fulfill(connectJSON(pending)),
	);
	await page.route('**/openchain.v1.TracingService/GetTraceStatus', (route) => {
		statusRequests++;
		return route.fulfill(connectJSON(completed));
	});
	await page.route('**/openchain.v1.LookupService/LookupAddress', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.route('**/openchain.v1.LabelService/GetLabels', (route) =>
		route.fulfill(connectJSON({})),
	);
	await page.route('**/openchain.v1.EvidenceService/ExportEvidencePackage', (route) => {
		exportRequests++;
		return route.fulfill(connectJSON({ packageJson: packageJSON }));
	});
	await page.goto('/');
	await page.getByPlaceholder('Search target address').fill(root);
	await page.getByLabel('Investigate address').click();
	await expect(page.getByText('Retrieving address flow…')).toBeVisible();
	await expect(page.getByText('Fan-out dispersion lead')).toBeVisible({ timeout: 10_000 });
	expect(statusRequests).toBe(1);
	const download = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Package' }).click();
	expect((await download).suggestedFilename()).toBe('openchain-evidence-package.json');
	expect(exportRequests).toBe(1);
	await page.locator('input[type=file]').setInputFiles({ name: 'evidence.json', mimeType: 'application/json', buffer: Buffer.from(packageJSON) });
	await expect(page.getByLabel('Case title')).toHaveValue('Deterministic replay');
	await expect(page.getByRole('button', { name: 'Print' })).toBeEnabled();
});

test('verifies and replays an imported frozen evidence package locally', async ({ page }) => {
	const root = '0x0000000000000000000000000000000000000001';
	const packageJSON = frozenEvidencePackage(root, 'ethereum-mainnet:tx:tx');
	await page.locator('input[type=file]').setInputFiles({ name: 'evidence.json', mimeType: 'application/json', buffer: Buffer.from(packageJSON) });
	await expect(page.getByLabel('Case title')).toHaveValue('Deterministic replay');
	await expect(page.getByRole('button', { name: 'Print' })).toBeEnabled();
});
