-- Database Initialization Script for OpenChain Platform (PostgreSQL + Apache AGE)

CREATE EXTENSION IF NOT EXISTS age;
LOAD 'age';
SET search_path = ag_catalog, "$user", public;

-- Create OpenChain graph namespace if it does not exist
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_graph WHERE name = 'openchain') THEN
        PERFORM create_graph('openchain');
    END IF;
END $$;

-- Create domain-specific vertex labels
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'Wallet' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
        PERFORM create_vlabel('openchain', 'Wallet');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'Contract' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
        PERFORM create_vlabel('openchain', 'Contract');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'Exchange' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
        PERFORM create_vlabel('openchain', 'Exchange');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'Label' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
        PERFORM create_vlabel('openchain', 'Label');
    END IF;
END $$;

-- Create domain-specific edge labels
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'TRANSFER' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
        PERFORM create_elabel('openchain', 'TRANSFER');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'MINT' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
        PERFORM create_elabel('openchain', 'MINT');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'SWAP' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
        PERFORM create_elabel('openchain', 'SWAP');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'HAS_LABEL' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
        PERFORM create_elabel('openchain', 'HAS_LABEL');
    END IF;
END $$;

-- Public Relational Tables for Cases, Shared Canvas, and Custom Labels
CREATE TABLE IF NOT EXISTS public.cases (
    id VARCHAR(64) PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    tags TEXT[],
    created_by VARCHAR(128) NOT NULL DEFAULT 'System',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS public.case_nodes (
    id SERIAL PRIMARY KEY,
    case_id VARCHAR(64) NOT NULL REFERENCES public.cases(id) ON DELETE CASCADE,
    address VARCHAR(64) NOT NULL,
    label VARCHAR(128),
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS public.case_edges (
    id SERIAL PRIMARY KEY,
    case_id VARCHAR(64) NOT NULL REFERENCES public.cases(id) ON DELETE CASCADE,
    source_address VARCHAR(64) NOT NULL,
    target_address VARCHAR(64) NOT NULL,
    tx_hash VARCHAR(66),
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS public.canvas_shares (
    share_id VARCHAR(64) PRIMARY KEY,
    snapshot_json JSONB NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS public.custom_labels (
    id VARCHAR(64) PRIMARY KEY,
    address VARCHAR(64) NOT NULL,
    network VARCHAR(32) NOT NULL DEFAULT 'ETHEREUM_SEPOLIA',
    category VARCHAR(64) NOT NULL,
    label VARCHAR(128) NOT NULL,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    evidence_url TEXT,
    source VARCHAR(64) NOT NULL DEFAULT 'USER',
    created_by VARCHAR(128) NOT NULL DEFAULT 'System',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_custom_labels_address ON public.custom_labels(LOWER(address));


