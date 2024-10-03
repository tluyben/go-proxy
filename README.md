# Go HTTP Proxy Server

This is a simple HTTP proxy server written in Go. It reads backend configurations from a YAML file, performs health checks on the backends, and forwards incoming requests to healthy backends.

## Features

- HTTP proxy (no HTTPS support)
- Configurable port, health check interval, and health check endpoint
- Regular health checks on backend servers
- Load balancing across healthy backends
- Fallback to other backends if one becomes unhealthy
- Returns a 502 Bad Gateway error if all backends are down

## Requirements

- Go 1.15 or higher
- `gopkg.in/yaml.v2` package

## Configuration

The proxy server is configured using a YAML file. By default, it looks for a file named `backend.yml` in the current directory. Here's an example configuration:

```yaml
port: 8080
interval: 5
health: /health
backends:
  - url: http://backend1.example.com
  - url: http://backend2.example.com
  - url: http://backend3.example.com
```

- `port`: The port on which the proxy server will listen (default: 80)
- `interval`: The interval in seconds between health checks (default: 3)
- `health`: The health check endpoint to use for all backends (default: /health)
- `backends`: A list of backend servers to proxy requests to

## Building and Running

This project includes a Makefile to simplify building and running the server.

To build the server:

```
make build
```

To run the server:

```
make run
```

To clean up build artifacts:

```
make clean
```

To run tests:

```
make test
```

To install dependencies:

```
make deps
```

## Running Manually

You can also run the server manually:

```
go run main.go --config backend.yml
```

Or, after building:

```
./proxy-server --config backend.yml
```

## Docker

To build a Docker image:

```
make docker-build
```

To run the Docker container:

```
docker run -p 8080:8080 proxy-server:latest
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License.
