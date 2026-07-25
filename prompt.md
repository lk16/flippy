
We're doing a full rewrite of flippy, which is a collection of tools related to the othello board game:
- db with pre-computed openings
- webserver that both serves rest api, websocket and a frontend
- worker that pre-computes positions

This is the initial prompt of the rewrite. You should translate this into AI readable instructions that create step plan such that we can implement everything 1-by-1. Use a checklist in a file for this, checking things off 1 by 1 after they are finished.



---

The old code is in git, but also available on this machine in the `old/` folder.
It contains a python and go implmentation of some parts that may contradict each other or be incomplete.
Be aware of this.

Eventually we want to remove the code in `old/` folder on this machine, so don't rely on it. Do not reference files there in code.

A lot of structure and implementation will change but we want to preserve almost all behaviour.
For that reason, in your output prompt, construct a section listing behaviour/features that are not mentioned in this file.
We will eventually go through them and either implement them or explicitly say we don't want them.

Behaviour in general:
- never assume something, always confirm with code first, otherwise with user
- keep responses concise
- prefer read tools over commands that need to be explictly approved by the user

Code best practices:
- use golang
- prefer standard library + minimal stable libraries otherwise
- write human readable code: no long functions, long arg lists, deep nesting
- all backend code except entrypoints must have unit tests
- when running integration tests against dockers, make sure this runs independent from local setup
- use minimal docstrings, one-liners are preferred
- always run `pre-commit run -a` and tests before each commit

Tools:
- golang + stdlib (templates, net/http)
- handroll basic othello types (see below)
- tests: gotestsum, testify
- docker compose for DB
- for DB table management use `migrate`
- edax (external evaluation tool)

Othello types:
- Board: stores white, black bitsets as 64 bit int + color to move
- Game: sequence of Board
- NormalizedBoard: wraps Board, but must be normalized.

Board normalization should work same as normalization logic in old python/go code, except we preserve the color to move.

Edax setup:
- Edax is an external binary, path taken from env
- See existing python/go code for how to integrate
- Avoid running one process per position, prefer 1 long running edax instance per worker
- Make sure that when worker shuts down we shutdown the edax process
- Edax evaluates boards differently depending on

Worker setup:
- loop forever:
    - get a Job from the API (Board + search level)
    - hand it to edax
    - wait for evaluation
    - post the Job + response back to the API

API:
- JSON REST api, prefixed with `api/`
- endpoints for getting Job + receiving Job responses
- endpoint for loading evaluation from DB from non-normalized Board

DB layout:
- store Board with disc count, search level, evaluation results
- index on Board will be crucial
- store evaluation of a position, do not store the best move

Frontend:
- simple admin website using go template library
- sidebar on left for selecting page
- pages:
    - main page:
        - show othello board with evaluations on squares
        - if evaluation is missing show an small dot in center with color of player to move
        - if evaluation is present show evaluation in center with color of player to move
        - handle passes
        - undo last move with right click on board
    - stats page:
        - show table with number of moves found: cols have level, rows have disc count.
        - cells with 0 should be empty
        - make sure we have an api endpoint that shows this info in one response
        - since board table will become big, it's important we index api endpoint behind this appropriately.
    - other pages may be added later

For open questions, suggestions are welcome. Some unstructured ideas/comments
- let workers send heartbeat to server periodically? to avoid jobs being claimed indefinitely
- we want to avoid computing things twice, make sure a job is claimed atomically
- if a worker goes down, its job should eventually become available for another worker
- we want to evaluate positions in order of disc count (lowest first), then current learn level (lowest first)
- do not learn positions with less than 12 discs. Precompute list of all 67245 NormalizedBoards and store in source.
- on server start, minimax all positions <11 discs from evaluations with 12 discs and put in a map.
- recompute this map if any evaluation of 12 discs is saved.
- do not learn positions with over 30 empties.

Loading files into `Game`:
- wtb: wthor files, see old golang code
- pgn: see example files in old
- othello quest style move sequence: string like this `A3B4C5`

Adding `NormalizedBoard`'s to DB:
- learning does not add or remove boards, only updates items.
- boards are never removed from DB
- boards can be added by use of special command, that imports files as sequence of `Game` (see above) and loads relevant `NormalizedBoard`'s from that.

Folder structure:
- repo root
    - cmd/{server,worker,loader}/main.go
    - internal/
        - web/
        - ... similar to go code in old
    - static/
    - db_data/ (postgres data, ignored)
    - pgn/ (pgn files, ignored)
    - wthor (wthor files, ignored)
