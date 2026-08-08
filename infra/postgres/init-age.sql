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

