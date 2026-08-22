# Build context is the repo root. Bakes the migrations into the upstream
# migrate CLI image; run with: -database "$FLIPPY_POSTGRES_URL" up
FROM migrate/migrate:v4.19.0
COPY migrations /migrations
ENTRYPOINT ["migrate", "-path", "/migrations"]
