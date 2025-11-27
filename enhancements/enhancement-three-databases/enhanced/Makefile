# Format the code using gofmt
neat:
	gofmt -w .

# Build the project
build:
	go build -o go-reverse-proxy ./cmd/go_reverse_proxy

# Run the application
run:
	go run ./cmd/go_reverse_proxy/main.go

# Database migrations
migrate-up:
	goose postgres "user=postgres dbname=reverse_proxy sslmode=disable" -dir db/migrations up

migrate-down:
	goose postgres "user=postgres dbname=reverse_proxy sslmode=disable" -dir db/migrations down

migrate-status:
	goose postgres "user=postgres dbname=reverse_proxy sslmode=disable" -dir db/migrations status

# Generate SQLC code
sqlc-generate:
	sqlc generate

.PHONY: neat build run migrate-up migrate-down migrate-status sqlc-generate