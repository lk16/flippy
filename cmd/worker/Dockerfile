# Build context is the repo root (see docker-compose.yml).

# Edax stage: clone a pinned commit and compile it for the target platform.
# This stage runs per-platform (under emulation when cross-building with
# buildx), since edax is C and its makefile picks flags per architecture.
FROM debian:bookworm-slim AS edax-build

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl gcc git libc6-dev make p7zip-full \
    && rm -rf /var/lib/apt/lists/*

ARG EDAX_REPO=https://github.com/lk16/edax-reversi
ARG EDAX_COMMIT=00f132ffe553649912777771f33b3725ab9c9975
RUN git clone --no-checkout "$EDAX_REPO" /edax-src \
    && git -C /edax-src checkout "$EDAX_COMMIT"

# ARCH=arm is the right makefile target for arm64: it adds no -march flags, and
# NEON (which edax's ARM code uses) is baseline on aarch64. ARCH=x64 is the
# portable amd64 build, matching the lEdax-x64 binary used in development.
ARG TARGETARCH
RUN case "$TARGETARCH" in \
    arm64) arch=arm ;; \
    amd64) arch=x64 ;; \
    *) echo "unsupported TARGETARCH: $TARGETARCH" >&2; exit 1 ;; \
    esac \
    && make -C /edax-src/src build ARCH="$arch" COMP=gcc OS=linux \
    && install -D "/edax-src/bin/lEdax-$arch" /edax/bin/edax

# The evaluation weights are not in the edax repo; fetch the release asset and
# pin the extracted file's hash. Edax resolves data/eval.dat relative to its
# working directory, which internal/edax sets to the directory above the
# binary's — /edax here, so /edax/data/eval.dat is found.
ARG EVAL_URL=https://github.com/abulmo/edax-reversi/releases/download/v4.4/eval.7z
ARG EVAL_SHA256=f8b2299612d9fa4414157e70e932636e33111c2602d0c2fc382a7d90ef21b792
RUN curl -fsSL "$EVAL_URL" -o /tmp/eval.7z \
    && 7z x -o/tmp/eval /tmp/eval.7z \
    && echo "$EVAL_SHA256  /tmp/eval/data/eval.dat" | sha256sum -c - \
    && install -D /tmp/eval/data/eval.dat /edax/data/eval.dat

# Go stage: cross-compiled on the build platform, so it never needs emulation.
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS go-build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
ARG TARGETOS TARGETARCH COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags "-X github.com/lk16/flippy/internal/version.Commit=$COMMIT" \
    -o /worker ./cmd/worker

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=edax-build /edax /edax
COPY --from=go-build /worker /usr/local/bin/worker
ENV EDAX_PATH=/edax/bin/edax
ENTRYPOINT ["/bin/sh", "-c", "exec /usr/local/bin/worker -edax-tasks=${EDAX_TASKS:-1}"]
