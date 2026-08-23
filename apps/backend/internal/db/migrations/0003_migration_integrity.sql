-- The historical migrations are immutable. These checksums are their exact
-- SHA-256 digests at the point migration integrity was introduced.
ALTER TABLE public.schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT;

UPDATE public.schema_migrations
SET checksum = CASE version
  WHEN '0001_initial' THEN 'b4aeac64f4fa42a928bdcced2800060ab19017d02d3421ea5dc076f03a2d022b'
  WHEN '0002_age_labels' THEN '3e3dee3dafc3a7f350cfe19bcf878ac5371782c471a24215e825a0ed2627027d'
END
WHERE checksum IS NULL;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.schema_migrations WHERE checksum IS NULL) THEN
    RAISE EXCEPTION 'cannot establish migration checksums for unknown migration history';
  END IF;
END $$;

ALTER TABLE public.schema_migrations ALTER COLUMN checksum SET NOT NULL;
