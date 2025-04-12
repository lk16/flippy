CREATE TABLE public.edax (
    position bytea NOT NULL,
    disc_count integer NOT NULL,
    level integer NOT NULL,
    depth integer NOT NULL,
    confidence integer NOT NULL,
    score integer NOT NULL,
    best_moves integer[] NOT NULL
);


CREATE INDEX idx_edax_disc_count ON public.edax USING btree (disc_count);
CREATE INDEX idx_edax_disc_count_level ON public.edax USING btree (disc_count, level);
CREATE INDEX idx_edax_learn_depth ON public.edax USING btree (depth);
CREATE INDEX idx_edax_level ON public.edax USING btree (level);
CREATE UNIQUE INDEX idx_edax_position ON public.edax USING btree (position);

CREATE MATERIALIZED VIEW public.edax_stats_view AS
SELECT
    disc_count,
    level,
    COUNT(*) as count
FROM public.edax
GROUP BY disc_count, level;

CREATE UNIQUE INDEX ON public.edax_stats_view (disc_count, level);
