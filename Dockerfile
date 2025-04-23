# ---- Build Stage ----
# Use an official Go image as the builder
FROM golang:1.21-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go module files
COPY go.mod go.sum ./
# Download dependencies
RUN go mod download

# Copy the source code
COPY *.go ./

# Build the Go app statically for Linux
# CGO_ENABLED=0 prevents linking against C libraries
# -ldflags="-w -s" strips debug information, making the binary smaller
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /reddit-monitor-app .

# ---- Final Stage ----
# Use a minimal distroless image for the final container
FROM gcr.io/distroless/base-debian11

# Set the working directory
WORKDIR /app

# Copy *only* the compiled binary from the builder stage
COPY --from=builder /reddit-monitor-app .

# Set the user (optional but good practice)
# Ensure the nonroot user exists in the base image (distroless/base has it)
USER nonroot:nonroot

# Command to run when the container starts
CMD ["/app/reddit-monitor-app"]