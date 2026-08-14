import { describe, expect, it } from 'vitest';
import { createLocalCase } from './case-file';
import {
	EVIDENCE_PACKAGE_FORMAT,
	EVIDENCE_PACKAGE_VERSION,
	parseEvidencePackage,
	replayEvidencePackage,
} from './evidence-package';

async function evidenceJSON() {
	const caseFile = createLocalCase();
	caseFile.rootAddress = '0x0000000000000000000000000000000000000001';
	const payload = {
		exported_at: '2026-08-12T00:00:00Z',
		case: caseFile,
		transfers: [
			{
				id: 'ethereum-mainnet:tx:tx',
				network: 'ethereum-mainnet',
				transaction_hash: '0xtx',
				event_id: 'tx',
				transfer_kind: 'NATIVE',
				from_address: caseFile.rootAddress,
				to_address: '0x0000000000000000000000000000000000000002',
				asset: { kind: 'NATIVE', contract_address: '', symbol: 'ETH', decimals: 18 },
				amount_base_units: '42',
				block_number: 1,
				block_hash: '0xblock',
				block_timestamp: '2026-08-11T00:00:00Z',
				provisional: false,
				source: 'test-provider',
				retrieved_at: '2026-08-12T00:00:00Z',
			},
		],
		acquisition_snapshots: [],
		acquisition_scopes: [],
		scope_transfers: [],
		scope_snapshots: [],
		rule_runs: [],
		labels: [],
		bridge_transitions: [],
		bridge_transition_acquisitions: [],
	};
	const hash = await crypto.subtle.digest(
		'SHA-256',
		new TextEncoder().encode(JSON.stringify(payload)),
	);
	const hex = [...new Uint8Array(hash)].map((byte) => byte.toString(16).padStart(2, '0')).join('');
	return JSON.stringify({
		format: EVIDENCE_PACKAGE_FORMAT,
		version: EVIDENCE_PACKAGE_VERSION,
		payload,
		manifest: { algorithm: 'SHA-256', payload_sha256: hex },
	});
}

describe('evidence packages', () => {
	it('verifies and replays frozen transfer evidence without a provider', async () => {
		const packageFile = await parseEvidencePackage(await evidenceJSON());
		const replay = replayEvidencePackage(packageFile);
		expect(replay.graph.edges).toHaveLength(1);
		expect(replay.graph.edges[0].sourceName).toBe('test-provider');
		expect(replay.graph.edges[0].provisional).toBe(false);
	});

	it('rejects altered payloads', async () => {
		const altered = JSON.parse(await evidenceJSON()) as {
			payload: { transfers: Array<{ amount_base_units: string }> };
		};
		altered.payload.transfers[0].amount_base_units = '43';
		await expect(parseEvidencePackage(JSON.stringify(altered))).rejects.toThrow('integrity');
	});

	it('rejects scope links that do not point to exported evidence', async () => {
		const inconsistent = JSON.parse(await evidenceJSON()) as {
			payload: { scope_transfers: unknown[] };
			manifest: { payload_sha256: string };
		};
		inconsistent.payload.scope_transfers = [{ scope_id: 9, transfer_id: 'missing' }];
		const hash = await crypto.subtle.digest(
			'SHA-256',
			new TextEncoder().encode(JSON.stringify(inconsistent.payload)),
		);
		inconsistent.manifest.payload_sha256 = [...new Uint8Array(hash)]
			.map((byte) => byte.toString(16).padStart(2, '0'))
			.join('');
		await expect(parseEvidencePackage(JSON.stringify(inconsistent))).rejects.toThrow('scope links');
	});
});
