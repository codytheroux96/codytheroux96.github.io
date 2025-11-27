# Go Reverse Proxy 

A localhost version of a dynamic reverse proxy in Go built using the standard library. 

## Features

- Listening for incoming HTTP(S) requests
- Dynamic routing to test backends based on registered route prefixes
- Wildcard-style forwarding based on client-facing paths (e.g. `/s1/*`, `/s2/*`)
- Forwarding HTTP requests to appropriate backends, preserving method, headers, and body
- Returning backend responses to clients
- Rate limiting to restrict how frequently clients can send requests
- In-memory caching for GET requests
- Error handling with retries if a backend fails
- Timeout handling to prevent hanging requests
- Substantial logging for observability
- HTTPS support with local certificates

## Project Structure

- `main.go`: Starts the proxy and both test servers
- `internal/app/`: Core logic for the proxy (routing, caching, rate limiting)
- `internal/registry/`: Registry logic for managing backend registration/deregistration
- `test_servers/server_one/`: A minimal backend responding to `/s1/*` routes
- `test_servers/server_two/`: A second backend for `/s2/*` routes

## Usage

Run the main application and the proxy will start on port `:8080`, with test backends on ports `:4200` and `:2200`.

Example routes to test:
- `GET /s1/health` - simple GET request with no substance
- `POST /s1/list` - simple POST request with no substance
- `GET /s2/health` - simple GET request with no substance
- `POST /s2/list` - simple POST request with no substance
- `POST /s1/echo` – echoes back the request body
- `POST /s2/echo` – echoes back the request body
- `GET /s1/headers` – returns request headers
- `GET /s2/headers` – returns request headers

For example: `http://localhost:8080/s2/headers` will call `http://localhost:2200/headers`

Example Usage With Curl:
```bash
curl -k https://localhost:8443/s2/headers 
```

## Notes

- This proxy only runs locally; it is **not deployed** and not accessible from outside your machine
- Both the proxy and the backend servers listen on `localhost`
- Test backends automatically register themselves with the proxy on startup and are automatically unregistered on shutdown.
- If you want to hit any of the routes yourself then you will need to either get rid of the HTTPS redirect logic in `main.go` or generate your own certificates for HTTPS support