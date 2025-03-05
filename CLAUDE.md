# Hazelnut Coding Guidelines

## Build & Test Commands
```bash
# Build the project
go build -o hazelnut

# Run linting
go vet ./...

# Format code
go fmt ./...

# Run tests (when implemented)
go test ./...

# Run a single test (when implemented)
go test ./path/to/package -run TestName
```

## Code Style

- **Imports**: Standard library first, then third-party packages, separated by blank line
- **Formatting**: Use `go fmt` for consistent formatting
- **Types**: Return concrete types from functions, accept interfaces as parameters
- **Naming**: CamelCase for exported names, camelCase for unexported, descriptive package names
- **Error Handling**: Wrap errors with context using `fmt.Errorf("context: %w", err)`
- **Logging**: Use structured logging with `slog`, add package name to logger context
- **Documentation**: Document all exported functions, types, and packages with godoc comments

## Project Structure
Hazelnut is a caching reverse proxy with these main components:
- `frontend`: HTTP server handling requests
- `backend`: Client communicating with origin servers
- `cache`: Storage using Ristretto caching library
- `config`: Configuration handling
- `metrics`: Prometheus metrics
