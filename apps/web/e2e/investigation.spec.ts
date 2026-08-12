import { expect, test } from '@playwright/test';

test.beforeEach(async ({ page }) => {
	await page.goto('/');
	await expect(page.getByPlaceholder('Search target address')).toBeVisible();
	await expect(page.getByLabel('Case title')).toBeVisible();
});

test('keeps EVM chain selection explicit and detects unique address formats', async ({ page }) => {
	const network = page.getByLabel('Network');
	const address = page.getByPlaceholder('Search target address');

	await network.selectOption({ label: 'Base Mainnet' });
	await address.fill('0x7a250d5630b4cf539739df2c5dacb4c659f2488d');
	await expect(network).toHaveValue('2');
	await expect(page.getByText('EVM address — choose its network')).toBeVisible();

	await address.fill('11111111111111111111111111111111');
	await expect(network).toHaveValue('3');

	await address.fill('TXLAQ63Xg1NAzckPwKHvzw7CSEmLMEqcdj');
	await expect(network).toHaveValue('4');
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

test('verifies and replays an imported frozen evidence package locally', async ({ page }) => {
	const packageJSON = await page.evaluate(async () => {
		const root = '0x0000000000000000000000000000000000000001';
		const payload = {
			exported_at: '2026-08-12T00:00:00Z',
			case: {
				version: 1, title: 'Frozen replay', network: 'ethereum-mainnet', createdAt: '2026-08-12T00:00:00Z', updatedAt: '2026-08-12T00:00:00Z', rootAddress: root, notes: 'No live provider is used.', selectedAddressIds: [], selectedTransferIds: ['ethereum-mainnet:tx:tx'], annotations: [],
			},
			transfers: [{ id: 'ethereum-mainnet:tx:tx', network: 'ethereum-mainnet', transaction_hash: '0xtx', event_id: 'tx', transfer_kind: 'NATIVE', from_address: root, to_address: '0x0000000000000000000000000000000000000002', asset: { kind: 'NATIVE', contract_address: '', symbol: 'ETH', decimals: 18 }, amount_base_units: '42', block_number: 1, block_hash: '0xblock', block_timestamp: '2026-08-11T00:00:00Z', provisional: false, source: 'frozen-test', retrieved_at: '2026-08-12T00:00:00Z' }],
			acquisition_snapshots: [], provenance: [], rule_runs: [], labels: [],
		};
		const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(JSON.stringify(payload)));
		const hash = [...new Uint8Array(digest)].map((value) => value.toString(16).padStart(2, '0')).join('');
		return JSON.stringify({ format: 'openchain-evidence-package', version: 1, payload, manifest: { algorithm: 'SHA-256', payload_sha256: hash } });
	});
	await page.locator('input[type=file]').setInputFiles({ name: 'evidence.json', mimeType: 'application/json', buffer: Buffer.from(packageJSON) });
	await expect(page.getByLabel('Case title')).toHaveValue('Frozen replay');
	await expect(page.getByRole('button', { name: 'Print' })).toBeEnabled();
});
