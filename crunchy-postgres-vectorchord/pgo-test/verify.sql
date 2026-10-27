SELECT 'vchord=' || extversion FROM pg_extension WHERE extname='vchord';
DO $$ BEGIN IF (SELECT count(*) FROM embeddings) <> 1000 THEN RAISE EXCEPTION 'row count mismatch'; END IF; END $$;
SET enable_seqscan = off;
SET vchordrq.probes = '1';
EXPLAIN (COSTS OFF) SELECT id FROM embeddings ORDER BY embedding <-> '[500,1,0]' LIMIT 1;
DO $$
DECLARE nearest integer; plan json;
BEGIN
    EXECUTE 'EXPLAIN (FORMAT JSON, COSTS OFF) SELECT id FROM embeddings ORDER BY embedding <-> ''[500,1,0]'' LIMIT 1' INTO plan;
    IF plan::text NOT LIKE '%embeddings_idx%' THEN
        RAISE EXCEPTION 'Vector index not used: %', plan;
    END IF;
    SELECT id INTO nearest FROM embeddings ORDER BY embedding <-> '[500,1,0]' LIMIT 1;
    IF nearest IS DISTINCT FROM 500 THEN
        RAISE EXCEPTION 'Unexpected nearest neighbor: %', nearest;
    END IF;
END;
$$;
