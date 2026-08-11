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
