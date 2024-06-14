CREATE TABLE public.edax (
    position bytea,
    disc_count integer,
    level integer,
    depth integer,
    confidence integer,
    score integer,
    learn_priority integer,
    best_moves integer[]
);


CREATE INDEX idx_edax_disc_count ON public.edax USING btree (disc_count);
CREATE INDEX idx_edax_disc_count_level ON public.edax USING btree (disc_count, level);
CREATE INDEX idx_edax_learn_depth ON public.edax USING btree (depth);
CREATE INDEX idx_edax_learn_priority ON public.edax USING btree (learn_priority);
CREATE INDEX idx_edax_level ON public.edax USING btree (level);
CREATE UNIQUE INDEX idx_edax_position ON public.edax USING btree (position);

CREATE TABLE public.greedy (
    position bytea,
    score integer,
    best_move integer
);

CREATE UNIQUE INDEX idx_greedy_position ON public.greedy USING btree (position);
