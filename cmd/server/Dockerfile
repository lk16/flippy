# Build context is the repo root. static/, wasm/edax-eval's dist and js, the
# HTML templates, and the precomputed 12-disc seed are all go:embed'ed, so they
# only need to be present in the build context, not in the runtime image.
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY static static
COPY wasm wasm
ARG TARGETOS TARGETARCH COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags "-X github.com/lk16/flippy/internal/version.Commit=$COMMIT" \
    -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /server /server
EXPOSE 7777
ENTRYPOINT ["/server"]
