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
  block_hash TEXT,
  block_timestamp TIMESTAMPTZ NOT NULL,
  provisional BOOLEAN NOT NULL DEFAULT TRUE,
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
ALTER TABLE public.transfers ADD COLUMN IF NOT EXISTS block_hash TEXT;
ALTER TABLE public.transfers ADD COLUMN IF NOT EXISTS provisional BOOLEAN NOT NULL DEFAULT TRUE;

CREATE INDEX IF NOT EXISTS transfers_from_network_idx ON public.transfers (network, from_address, block_number DESC);
CREATE INDEX IF NOT EXISTS transfers_to_network_idx ON public.transfers (network, to_address, block_number DESC);

CREATE TABLE IF NOT EXISTS public.acquisition_blobs (
  response_sha256 TEXT PRIMARY KEY CHECK (length(response_sha256) = 64),
  response_body BYTEA NOT NULL
);

CREATE TABLE IF NOT EXISTS public.acquisition_snapshots (
  id BIGSERIAL PRIMARY KEY,
  network TEXT NOT NULL,
  provider TEXT NOT NULL,
  request_identity TEXT NOT NULL,
  response_sha256 TEXT NOT NULL CHECK (length(response_sha256) = 64),
  retrieved_at TIMESTAMPTZ NOT NULL
);

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'acquisition_snapshots' AND column_name = 'response_body'
  ) THEN
    EXECUTE 'INSERT INTO public.acquisition_blobs (response_sha256, response_body) SELECT response_sha256, response_body FROM public.acquisition_snapshots ON CONFLICT (response_sha256) DO NOTHING';
    EXECUTE 'ALTER TABLE public.acquisition_snapshots DROP COLUMN response_body';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'acquisition_snapshots_response_sha256_fkey' AND conrelid = 'public.acquisition_snapshots'::regclass
  ) THEN
    ALTER TABLE public.acquisition_snapshots
      ADD CONSTRAINT acquisition_snapshots_response_sha256_fkey
      FOREIGN KEY (response_sha256) REFERENCES public.acquisition_blobs(response_sha256);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS acquisition_snapshots_network_retrieved_idx ON public.acquisition_snapshots (network, retrieved_at DESC);
CREATE INDEX IF NOT EXISTS acquisition_snapshots_response_hash_idx ON public.acquisition_snapshots (response_sha256);

DROP TABLE IF EXISTS public.transfer_acquisitions;

CREATE TABLE IF NOT EXISTS public.acquisition_scopes (
  id BIGSERIAL PRIMARY KEY,
  network TEXT NOT NULL,
  address TEXT NOT NULL,
  cursor TEXT NOT NULL,
  retrieved_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS acquisition_scopes_lookup_idx ON public.acquisition_scopes (network, address, retrieved_at DESC);

CREATE TABLE IF NOT EXISTS public.acquisition_scope_transfers (
  scope_id BIGINT NOT NULL REFERENCES public.acquisition_scopes(id),
  transfer_id TEXT NOT NULL REFERENCES public.transfers(id),
  PRIMARY KEY (scope_id, transfer_id)
);

CREATE INDEX IF NOT EXISTS acquisition_scope_transfers_transfer_idx ON public.acquisition_scope_transfers (transfer_id);

CREATE TABLE IF NOT EXISTS public.acquisition_scope_snapshots (
  scope_id BIGINT NOT NULL REFERENCES public.acquisition_scopes(id),
  acquisition_id BIGINT NOT NULL REFERENCES public.acquisition_snapshots(id),
  PRIMARY KEY (scope_id, acquisition_id)
);

CREATE INDEX IF NOT EXISTS acquisition_scope_snapshots_acquisition_idx ON public.acquisition_scope_snapshots (acquisition_id);

CREATE OR REPLACE FUNCTION public.reject_evidence_mutation() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'evidence acquisition records are immutable';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS acquisition_snapshots_immutable ON public.acquisition_snapshots;
CREATE TRIGGER acquisition_snapshots_immutable BEFORE UPDATE OR DELETE ON public.acquisition_snapshots FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
DROP TRIGGER IF EXISTS acquisition_blobs_immutable ON public.acquisition_blobs;
CREATE TRIGGER acquisition_blobs_immutable BEFORE UPDATE OR DELETE ON public.acquisition_blobs FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
DROP TRIGGER IF EXISTS acquisition_scopes_immutable ON public.acquisition_scopes;
CREATE TRIGGER acquisition_scopes_immutable BEFORE UPDATE OR DELETE ON public.acquisition_scopes FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
DROP TRIGGER IF EXISTS acquisition_scope_transfers_immutable ON public.acquisition_scope_transfers;
CREATE TRIGGER acquisition_scope_transfers_immutable BEFORE UPDATE OR DELETE ON public.acquisition_scope_transfers FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
DROP TRIGGER IF EXISTS acquisition_scope_snapshots_immutable ON public.acquisition_scope_snapshots;
CREATE TRIGGER acquisition_scope_snapshots_immutable BEFORE UPDATE OR DELETE ON public.acquisition_scope_snapshots FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();

CREATE TABLE IF NOT EXISTS public.bridge_transitions (
  id TEXT PRIMARY KEY,
  protocol TEXT NOT NULL,
  bridge_name TEXT NOT NULL,
  source_network TEXT NOT NULL,
  destination_network TEXT NOT NULL,
  lifecycle TEXT NOT NULL CHECK (lifecycle IN ('initiated', 'relayed', 'finalized', 'failed', 'unresolved')),
  message_id TEXT NOT NULL,
  source_transfer_id TEXT NOT NULL REFERENCES public.transfers(id),
  destination_transfer_id TEXT REFERENCES public.transfers(id),
  source_transaction_hash TEXT NOT NULL,
  destination_transaction_hash TEXT,
  source_log_reference TEXT NOT NULL,
  destination_log_reference TEXT,
  source_bridge_address TEXT NOT NULL,
  destination_bridge_address TEXT NOT NULL,
  canonical_source_token TEXT NOT NULL,
  canonical_destination_token TEXT NOT NULL,
  recipient TEXT NOT NULL,
  asset_kind TEXT NOT NULL,
  asset_contract_address TEXT NOT NULL,
  asset_symbol TEXT NOT NULL,
  asset_decimals INTEGER NOT NULL,
  amount_base_units TEXT NOT NULL,
  source_block_number BIGINT NOT NULL,
  destination_block_number BIGINT,
  source_block_hash TEXT NOT NULL,
  destination_block_hash TEXT,
  source_timestamp TIMESTAMPTZ NOT NULL,
  destination_timestamp TIMESTAMPTZ,
  source_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
  destination_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
  limitations TEXT NOT NULL,
  observed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS bridge_transitions_source_transfer_idx ON public.bridge_transitions (source_transfer_id, source_timestamp DESC);
CREATE INDEX IF NOT EXISTS bridge_transitions_message_idx ON public.bridge_transitions (protocol, message_id, observed_at DESC);

CREATE TABLE IF NOT EXISTS public.bridge_transition_acquisitions (
  transition_id TEXT NOT NULL REFERENCES public.bridge_transitions(id),
  side TEXT NOT NULL CHECK (side IN ('source', 'destination')),
  acquisition_id BIGINT NOT NULL REFERENCES public.acquisition_snapshots(id),
  PRIMARY KEY (transition_id, side, acquisition_id)
);

CREATE INDEX IF NOT EXISTS bridge_transition_acquisitions_snapshot_idx ON public.bridge_transition_acquisitions (acquisition_id);
DROP TRIGGER IF EXISTS bridge_transitions_immutable ON public.bridge_transitions;
CREATE TRIGGER bridge_transitions_immutable BEFORE UPDATE OR DELETE ON public.bridge_transitions FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
DROP TRIGGER IF EXISTS bridge_transition_acquisitions_immutable ON public.bridge_transition_acquisitions;
CREATE TRIGGER bridge_transition_acquisitions_immutable BEFORE UPDATE OR DELETE ON public.bridge_transition_acquisitions FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();

CREATE TABLE IF NOT EXISTS public.rule_catalog (
  rule_id TEXT NOT NULL,
  version TEXT NOT NULL,
  name TEXT NOT NULL,
  parameter_schema JSONB NOT NULL,
  default_parameters JSONB NOT NULL,
  limitations TEXT NOT NULL,
  PRIMARY KEY (rule_id, version)
);

CREATE TABLE IF NOT EXISTS public.rule_runs (
  id BIGSERIAL PRIMARY KEY,
  network TEXT NOT NULL,
  rule_id TEXT NOT NULL,
  rule_version TEXT NOT NULL,
  parameters JSONB NOT NULL,
  input_transfer_ids JSONB NOT NULL,
  result JSONB NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ NOT NULL,
  FOREIGN KEY (rule_id, rule_version) REFERENCES public.rule_catalog (rule_id, version)
);

CREATE INDEX IF NOT EXISTS rule_runs_network_completed_idx ON public.rule_runs (network, completed_at DESC);

CREATE TABLE IF NOT EXISTS public.trace_jobs (
  id BIGSERIAL PRIMARY KEY,
  network TEXT NOT NULL,
  address TEXT NOT NULL,
  direction TEXT NOT NULL,
  cursor TEXT NOT NULL,
  page_size INTEGER NOT NULL,
  counterparty_limit INTEGER NOT NULL DEFAULT 10,
  ranking TEXT NOT NULL DEFAULT 'most_recent',
	max_depth INTEGER NOT NULL DEFAULT 1,
  client_key TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  result_json JSONB,
  error_message TEXT,
  lease_expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
	UNIQUE (network, address, direction, cursor, page_size, counterparty_limit, ranking, max_depth)
);

ALTER TABLE public.trace_jobs ADD COLUMN IF NOT EXISTS client_key TEXT NOT NULL DEFAULT '';
ALTER TABLE public.trace_jobs ADD COLUMN IF NOT EXISTS counterparty_limit INTEGER NOT NULL DEFAULT 10;
ALTER TABLE public.trace_jobs ADD COLUMN IF NOT EXISTS ranking TEXT NOT NULL DEFAULT 'most_recent';
ALTER TABLE public.trace_jobs ADD COLUMN IF NOT EXISTS max_depth INTEGER NOT NULL DEFAULT 1;
UPDATE public.trace_jobs SET completed_at = updated_at WHERE status = 'failed' AND completed_at IS NULL;
ALTER TABLE public.trace_jobs DROP CONSTRAINT IF EXISTS trace_jobs_network_address_direction_cursor_page_size_key;
ALTER TABLE public.trace_jobs DROP CONSTRAINT IF EXISTS trace_jobs_network_address_direction_cursor_page_size_count_key;
DROP INDEX IF EXISTS public.trace_jobs_query_idx;
CREATE UNIQUE INDEX trace_jobs_query_idx ON public.trace_jobs (network, address, direction, cursor, page_size, counterparty_limit, ranking, max_depth);

CREATE INDEX IF NOT EXISTS trace_jobs_queued_idx ON public.trace_jobs (created_at) WHERE status = 'queued';
DROP INDEX IF EXISTS trace_jobs_client_queued_idx;
CREATE INDEX IF NOT EXISTS trace_jobs_network_client_queued_idx ON public.trace_jobs (network, client_key) WHERE status = 'queued';
CREATE INDEX IF NOT EXISTS trace_jobs_finished_retention_idx ON public.trace_jobs (network, completed_at) WHERE status IN ('succeeded', 'failed');
DROP INDEX IF EXISTS public.trace_jobs_one_running_idx;
CREATE UNIQUE INDEX IF NOT EXISTS trace_jobs_one_running_per_network_idx ON public.trace_jobs (network) WHERE status = 'running';

DROP TABLE IF EXISTS public.curated_labels;

CREATE TABLE IF NOT EXISTS public.label_evidence (
  sha256 TEXT PRIMARY KEY CHECK (length(sha256) = 64),
  content BYTEA NOT NULL,
  captured_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS public.label_assertions (
  id TEXT PRIMARY KEY,
  assertion_key TEXT NOT NULL,
  network TEXT NOT NULL,
  address TEXT NOT NULL,
  category TEXT NOT NULL,
  label TEXT NOT NULL,
  confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  evidence_sha256 TEXT NOT NULL REFERENCES public.label_evidence(sha256),
  evidence_url TEXT NOT NULL,
  source TEXT NOT NULL,
  source_version TEXT NOT NULL,
  supersedes_id TEXT REFERENCES public.label_assertions(id),
  review_state TEXT NOT NULL CHECK (review_state IN ('approved', 'rejected', 'pending')),
  visibility TEXT NOT NULL CHECK (visibility = 'public'),
  trust_tier SMALLINT NOT NULL CHECK (trust_tier BETWEEN 1 AND 3),
  valid_from TIMESTAMPTZ NOT NULL,
  valid_to TIMESTAMPTZ,
  created_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CHECK (valid_to IS NULL OR valid_to > valid_from),
  UNIQUE (assertion_key, source_version)
);

ALTER TABLE public.label_assertions ADD COLUMN IF NOT EXISTS supersedes_id TEXT;
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'label_assertions_supersedes_id_fkey' AND conrelid = 'public.label_assertions'::regclass
  ) THEN
    ALTER TABLE public.label_assertions
      ADD CONSTRAINT label_assertions_supersedes_id_fkey
      FOREIGN KEY (supersedes_id) REFERENCES public.label_assertions(id);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS label_assertions_current_address_idx ON public.label_assertions (network, address, valid_from DESC) WHERE review_state = 'approved';
CREATE INDEX IF NOT EXISTS label_assertions_current_category_idx ON public.label_assertions (network, category, valid_from DESC) WHERE review_state = 'approved';
CREATE UNIQUE INDEX IF NOT EXISTS label_assertions_supersedes_idx ON public.label_assertions (supersedes_id) WHERE supersedes_id IS NOT NULL;

DROP TRIGGER IF EXISTS label_evidence_immutable ON public.label_evidence;
CREATE TRIGGER label_evidence_immutable BEFORE UPDATE OR DELETE ON public.label_evidence FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
DROP TRIGGER IF EXISTS label_assertions_immutable ON public.label_assertions;
CREATE TRIGGER label_assertions_immutable BEFORE UPDATE OR DELETE ON public.label_assertions FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
