CREATE EXTENSION IF NOT EXISTS age;
LOAD 'age';
SET LOCAL search_path = ag_catalog, "$user", public;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_graph WHERE name = 'openchain') THEN
    PERFORM create_graph('openchain');
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS public.transfers (
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

CREATE INDEX IF NOT EXISTS transfers_from_network_idx ON public.transfers (network, from_address, block_number DESC);
CREATE INDEX IF NOT EXISTS transfers_to_network_idx ON public.transfers (network, to_address, block_number DESC);

CREATE TABLE IF NOT EXISTS public.trace_jobs (
  id BIGSERIAL PRIMARY KEY,
  network TEXT NOT NULL,
  address TEXT NOT NULL,
  direction TEXT NOT NULL,
  cursor TEXT NOT NULL,
  page_size INTEGER NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  result_json JSONB,
  error_message TEXT,
  lease_expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE (network, address, direction, cursor, page_size)
);

CREATE INDEX IF NOT EXISTS trace_jobs_queued_idx ON public.trace_jobs (created_at) WHERE status = 'queued';
CREATE UNIQUE INDEX IF NOT EXISTS trace_jobs_one_running_idx ON public.trace_jobs (status) WHERE status = 'running';
