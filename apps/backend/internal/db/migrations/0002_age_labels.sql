-- Graph labels are schema objects in Apache AGE. Create them with the
-- migration owner so the restricted runtime role only writes observations.
LOAD 'age';
SET LOCAL search_path = ag_catalog, "$user", public;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM ag_catalog.ag_label label
    JOIN ag_catalog.ag_graph graph ON graph.graphid = label.graph
    WHERE graph.name = 'openchain' AND label.name = 'Address' AND label.kind = 'v'
  ) THEN
    PERFORM create_vlabel('openchain', 'Address');
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM ag_catalog.ag_label label
    JOIN ag_catalog.ag_graph graph ON graph.graphid = label.graph
    WHERE graph.name = 'openchain' AND label.name = 'FundFlow' AND label.kind = 'e'
  ) THEN
    PERFORM create_elabel('openchain', 'FundFlow');
  END IF;
END $$;
