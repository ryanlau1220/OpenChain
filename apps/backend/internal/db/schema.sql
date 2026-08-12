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
  event_id TEXT NOT NULL,
  transfer_kind TEXT NOT NULL,
  from_address TEXT NOT NULL,
  to_address TEXT NOT NULL,
  asset_symbol TEXT NOT NULL,
  asset_kind TEXT NOT NULL,
  asset_contract_address TEXT NOT NULL,
  asset_decimals INTEGER NOT NULL,
  amount_base_units TEXT NOT NULL,
  block_number BIGINT NOT NULL,
  block_timestamp TIMESTAMPTZ NOT NULL,
  source TEXT NOT NULL,
  retrieved_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS public.assets (
  network TEXT NOT NULL,
  contract_address TEXT NOT NULL,
  kind TEXT NOT NULL,
  symbol TEXT NOT NULL,
  decimals INTEGER NOT NULL,
  retrieved_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (network, contract_address)
);

ALTER TABLE public.transfers DROP COLUMN IF EXISTS event_index;
ALTER TABLE public.transfers ADD COLUMN IF NOT EXISTS event_id TEXT;
ALTER TABLE public.transfers ADD COLUMN IF NOT EXISTS transfer_kind TEXT;
ALTER TABLE public.transfers ADD COLUMN IF NOT EXISTS asset_kind TEXT;
ALTER TABLE public.transfers ADD COLUMN IF NOT EXISTS asset_contract_address TEXT;
ALTER TABLE public.transfers ADD COLUMN IF NOT EXISTS asset_decimals INTEGER;

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
DROP INDEX IF EXISTS public.trace_jobs_one_running_idx;
CREATE UNIQUE INDEX IF NOT EXISTS trace_jobs_one_running_per_network_idx ON public.trace_jobs (network) WHERE status = 'running';

CREATE TABLE IF NOT EXISTS public.curated_labels (
  id TEXT PRIMARY KEY,
  network TEXT NOT NULL,
  address TEXT NOT NULL,
  category TEXT NOT NULL,
  label TEXT NOT NULL,
  confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  evidence_url TEXT NOT NULL,
  source TEXT NOT NULL,
  source_version TEXT NOT NULL,
  visibility TEXT NOT NULL CHECK (visibility = 'public'),
  trust_tier SMALLINT NOT NULL CHECK (trust_tier BETWEEN 1 AND 3),
  created_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS curated_labels_address_idx ON public.curated_labels (network, address);
CREATE INDEX IF NOT EXISTS curated_labels_category_idx ON public.curated_labels (category);
