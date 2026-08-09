CREATE EXTENSION IF NOT EXISTS age;
LOAD 'age';
SET search_path = ag_catalog, "$user", public;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_graph WHERE name = 'openchain') THEN
    PERFORM create_graph('openchain');
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS transfers (
  id TEXT PRIMARY KEY,
  network TEXT NOT NULL,
  transaction_hash TEXT NOT NULL,
  event_index INTEGER NOT NULL,
  from_address TEXT NOT NULL,
  to_address TEXT NOT NULL,
  asset_symbol TEXT NOT NULL,
  amount_base_units TEXT NOT NULL,
  block_number BIGINT NOT NULL,
  block_timestamp TIMESTAMPTZ NOT NULL,
  source TEXT NOT NULL,
  retrieved_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS transfers_from_network_idx ON transfers (network, from_address, block_number DESC);
CREATE INDEX IF NOT EXISTS transfers_to_network_idx ON transfers (network, to_address, block_number DESC);
