setup:
	@go mod tidy
	@go install github.com/riverqueue/river/cmd/river@v0.30.2
check:
	@go mod tidy
	@go tool swag init -q --output ./swagger --generalInfo ./core/app.go
	@go build -o ./tmp/out . && find tmp -name '*out' -print0 | xargs -0 ls -lhS
	@golangci-lint run ./...
	@go tool sqlc diff
config:
	@go run . -e .env config
fmt:
	@go fmt ./...
	@golangci-lint fmt ./...
gen:
	@go generate ./...
dev:
	@go tool air
db:
	@psql "postgres://wobsongocore:LocalDev123@localhost:45432/wobsongocore_db?sslmode=disable"
dbup:
	docker compose up -d --wait
dbstop:
	docker compose stop
dbdown:
	docker compose down --remove-orphans --volumes
reset:
	@go run . -e .env reset
admin:
	@go run . -e .env createsuperadmin --email admin.com --username admin --password LocalDev123 --apply
fake:
	@go run . -e .env createsuperadmin --email admin.com --username admin --password LocalDev123 --apply
migrateup:
	@go run . -e .env migrateup
migratedown:
	@go run . -e .env migratedown
jwt:
	@go run . -e .env jwt
dball:
	@make dbdown; make dbup; make migrateup; make dbup
dbtestup:
	@docker compose -f test-compose.yaml down --remove-orphans --volumes
	@docker compose -f test-compose.yaml up --wait
	@psql "postgres://wobsongocoretest:LocalDev123@localhost:45433/wobsongocore_db?sslmode=disable" \
		-c "CREATE USER wobsongocore_user WITH PASSWORD 'LocalDev123';"
	@psql "postgres://wobsongocoretest:LocalDev123@localhost:45433/wobsongocore_db?sslmode=disable&connect_timeout=30" \
		-c "ALTER DATABASE wobsongocore_db OWNER TO wobsongocore_user;"
	# CREATE EXTENSION needs superuser (pgvector isn't marked "trusted"), and
	# wobsongocore_user isn't one — pre-create it as the bootstrap superuser
	# so the migration's own CREATE EXTENSION IF NOT EXISTS is a no-op.
	@psql "postgres://wobsongocoretest:LocalDev123@localhost:45433/wobsongocore_db?sslmode=disable&connect_timeout=30" \
		-c "CREATE EXTENSION IF NOT EXISTS vector;"
	@go run . -e test.env reset && go run . -e test.env migrateup
dbtestdown:
	@docker compose -f test-compose.yaml down --remove-orphans --volumes

# Run only unit tests (skip integration tests with -short flag)
test-unit:
	@echo "Running unit tests..."
	go test -short -v ./...

# Run only integration tests (requires test database)
test-integration:
	@echo "Running integration tests (requires test database)..."
	APP_ENV="testing" \
	APP_JWT_SECRET="VerySecretToken123456" \
	APP_JWT_EXPIRY_HOURS="2" \
	APP_DB_URI="postgres://wobsongocore_user:LocalDev123@localhost:45433/wobsongocore_db?sslmode=disable&connect_timeout=30&timezone=UTC" \
	APP_LOG_LEVEL="2" \
	go test -v ./internal/repo/...

# Run integration tests with race detector
test-integration-race:
	@echo "Running integration tests with race detector..."
	APP_ENV="testing" \
	APP_JWT_SECRET="VerySecretToken123456" \
	APP_JWT_EXPIRY_HOURS="2" \
	APP_DB_URI="postgres://wobsongocore_user:LocalDev123@localhost:45433/wobsongocore_db?sslmode=disable&connect_timeout=30&timezone=UTC" \
	APP_LOG_LEVEL="2" \
	go test -race -v ./internal/repo/...

# Run all tests with coverage
# Make sure to run `dbtestup` first to set up the test database
test:
	APP_ENV="testing" \
	APP_JWT_SECRET="VerySecretToken123456" \
	APP_JWT_EXPIRY_HOURS="2" \
	APP_DB_URI="postgres://wobsongocore_user:LocalDev123@localhost:45433/wobsongocore_db?sslmode=disable&connect_timeout=30&timezone=UTC" \
	APP_LOG_LEVEL="2" \
	go test \
		-count=1 \
		-coverprofile=coverdb.profile \
		-coverpkg=$$(go list ./... | grep -v -E '/view/|/components/' | paste -sd, -) \
		-covermode count ./... && \
		go tool cover -func coverdb.profile

testrace:
	APP_ENV="testing" \
	APP_JWT_SECRET="VerySecretToken123456" \
	APP_JWT_EXPIRY_HOURS="2" \
	APP_DB_URI="postgres://wobsongocore_user:LocalDev123@localhost:45433/wobsongocore_db?sslmode=disable&connect_timeout=30&timezone=UTC" \
	APP_LOG_LEVEL="2" \
	go test -race ./...

fecheck:
	cd frontend && pnpm run format && pnpm run check && pnpm run lint && pnpm run test
