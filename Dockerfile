# Stage 1: Build the Go application
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Initialize go module. This is required for `go build` in recent Go versions.
RUN go mod init prometheus-metrics-combiner

# Copy the source code into the container
COPY main.go .

# Build the Go app, creating a static binary.
# The -ldflags="-w -s" flag reduces the binary size.
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-w -s" -o /app/server .

# Stage 2: Create the final, minimal image
FROM alpine:latest

# Copy the static binary from the builder stage
COPY --from=builder /app/server /server

# Expose the port the app runs on
EXPOSE 8080

# Set the entrypoint. The user will provide flags like -url and -port on `docker run`.
ENTRYPOINT ["/server"]