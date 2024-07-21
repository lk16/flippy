# Use the official Python image from the Docker Hub with the specific version 3.12.2
FROM python:3.12.2-slim

# Set environment variables to ensure PDM works properly
ENV PDM_HOME=/root/.pdm \
    PDM_IGNORE_SAVED_PYTHON=1 \
    PIP_NO_CACHE_DIR=off \
    PIP_DISABLE_PIP_VERSION_CHECK=on \
    PYTHONUNBUFFERED=1

# Install necessary build dependencies
# TODO git and p7zip-full are only used to build edax-reversi
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    build-essential \
    curl \
    git \
    p7zip-full && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /

# TODO don't clone in build
RUN git clone https://github.com/abulmo/edax-reversi

# TODO use separate build image
# TODO don't hardcode target OS
RUN mkdir -p /edax-reversi/bin && \
    cd /edax-reversi/src && \
    make build ARCH=$EDAX_ARCH COMP=gcc OS=linux

# Download edax weights. TODO don't do this in the future
RUN cd /edax-reversi && \
    curl -OL https://github.com/abulmo/edax-reversi/releases/download/v4.4/eval.7z && \
    7z x eval.7z && \
    rm eval.7z

# Install PDM
RUN curl -sSL https://raw.githubusercontent.com/pdm-project/pdm/main/install-pdm.py | python3 -

# Add PDM to PATH
ENV PATH=$PATH:/root/.pdm/bin

# Create and set the working directory
WORKDIR /app

# Copy the PDM project files
COPY pyproject.toml pdm.lock /app/

# Install the dependencies using PDM
RUN pdm install

# Copy app source
COPY src/ /app/src/

# Install commands
RUN pdm install

WORKDIR /app

# Set the entry point to run the book-worker using PDM
ENTRYPOINT ["pdm", "run", "book-worker", "run"]
