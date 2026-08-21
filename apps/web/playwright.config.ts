import { defineConfig } from '@playwright/test';

export default defineConfig({
	testDir: './e2e',
	timeout: 15_000,
	use: {
		baseURL: process.env.OPENCHAIN_E2E_BASE_URL || 'http://127.0.0.1:3002',
		trace: 'retain-on-failure',
		screenshot: 'only-on-failure',
	},
});
