CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS vchord CASCADE;
CREATE EXTENSION IF NOT EXISTS earthdistance CASCADE;
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
SELECT extname, extversion FROM pg_extension ORDER BY extname;
DROP TABLE IF EXISTS embeddings;
CREATE TABLE embeddings (id integer PRIMARY KEY, embedding vector(3));
INSERT INTO embeddings
SELECT i, ARRAY[i::real, 1::real, 0::real]::vector
FROM generate_series(1, 1000) AS i;
CREATE INDEX embeddings_idx ON embeddings USING vchordrq (embedding vector_l2_ops)
WITH (options = $$[build.internal]
lists = [1]
spherical_centroids = false
$$);
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
