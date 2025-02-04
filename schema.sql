CREATE TABLE public.edax (
    position bytea,
    disc_count integer,
    level integer,
    depth integer,
    confidence integer,
    score integer,
    best_moves integer[]
);


CREATE INDEX idx_edax_disc_count ON public.edax USING btree (disc_count);
CREATE INDEX idx_edax_disc_count_level ON public.edax USING btree (disc_count, level);
CREATE INDEX idx_edax_learn_depth ON public.edax USING btree (depth);
CREATE INDEX idx_edax_level ON public.edax USING btree (level);
CREATE UNIQUE INDEX idx_edax_position ON public.edax USING btree (position);


CREATE TABLE public.edax_stats (
    disc_count integer NOT NULL,
    level integer NOT NULL,
    count bigint NOT NULL,
    PRIMARY KEY (disc_count, level)
);
