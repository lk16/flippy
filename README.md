# Flippy

Tools for learning and playing the othello boardgame, currently using pygame.


### How to use GUI
* Install python 3.11
* Install dependencies with `pdm install`
* Run with `pdm run gui`

### How to use docker openings book
* Start docker: `docker compose up -d`
* (Optional) Connect with postgres `psql 'postgres://book_user:book_pass@localhost:12321/flippy'`
* Create tables: `psql 'postgres://book_user:book_pass@localhost:12321/flippy' < schema.sql`
* Import book: `psql 'postgres://book_user:book_pass@localhost:12321/flippy' < dump_file.sql`
* Save book to file: `./archive_book.sh`

### How to learn book
* Run book server: `pdm run book server`
* Run learning client: `pdm run book client`
