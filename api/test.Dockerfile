FROM golang:1.24.2

WORKDIR /app

# Set up Go build cache
ENV GOCACHE=/go/cache

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the application
RUN go build -o /flippy_server ./cmd/server/main.go

# Expose the application port
EXPOSE 3000

# Run the application
CMD ["/flippy_server"]
