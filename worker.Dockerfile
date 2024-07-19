# Use the official Python image from the Docker Hub with the specific version 3.12.2
FROM python:3.12.2-slim

# Set environment variables to ensure PDM works properly
ENV PDM_HOME=/root/.pdm \
    PDM_IGNORE_SAVED_PYTHON=1 \
    PIP_NO_CACHE_DIR=off \
    PIP_DISABLE_PIP_VERSION_CHECK=on

# Install necessary build dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    build-essential \
    curl \
    && apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install PDM
RUN curl -sSL https://raw.githubusercontent.com/pdm-project/pdm/main/install-pdm.py | python3 -

# Add PDM to PATH
ENV PATH=$PATH:/root/.pdm/bin

# Create and set the working directory
WORKDIR /app

# Copy the PDM project files
COPY pyproject.toml pdm.lock /app/
COPY src/ /app/src/

# Install the dependencies using PDM
RUN pdm install

# Set the entry point to run the book-worker using PDM
ENTRYPOINT ["pdm", "run", "book-worker", "run"]
