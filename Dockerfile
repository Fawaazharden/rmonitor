# Use a minimal base image
FROM gcr.io/distroless/base-debian11 AS final

# Copy the compiled binary from your local machine into the image
WORKDIR /app
COPY reddit_monitor_linux .

# Set the user (optional but good practice)
USER nonroot:nonroot

# Command to run when the container starts
# Note: We rely on Render's environment variables for configuration
CMD ["/app/reddit_monitor_linux"]