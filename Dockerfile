# Start from debian:bullseye-slim
FROM debian:bullseye-slim

# Install necessary tools
RUN apt-get update && apt-get install -y \
    git \
    golang \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Clone the repository
RUN git clone https://github.com/tluyben/go-proxy.git .

# Build the application
RUN go build -o proxy-server

# Copy the backend.yml file
COPY backend.yml /backend.yml

# Expose the default port (assuming it's 80, adjust if different)
EXPOSE 80

# Run the proxy server
CMD ["./proxy-server", "--config", "/backend.yml"]
