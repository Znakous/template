-include .env
-include .env.local

export

.PHONY: migrate migrate-down migrate-status run test test-integration lint generate check healthcheck new-migration

migrate:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" down

migrate-status:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" status

run:
	go run ./cmd/trip-service

test:
	go test -race -count=2 -coverprofile=coverage.out ./cmd/... ./internal/...
	go tool cover -func=coverage.out | tail -n 1

test-integration:
	go test -race -count=1 -tags integration ./internal/repository/...

lint:
	golangci-lint run

check:
	sh tools/check.sh

healthcheck:
	sh tools/healthcheck.sh

new-migration:
	sh tools/goose.sh

generate:
	go tool oapi-codegen -generate types,chi-server -include-operation-ids createTrip,getTrip,finishTrip,health,ready -package api -o internal/generated/api.gen.go contracts/openapi/trip-service.openapi.yaml
	go generate ./...

kill:
	@pkill -f "trip-service" || true

docker:
	docker run -d \
      --name trip-service \
      -p 8080:8080 \
      --env-file .env \
      --add-host=host.docker.internal:host-gateway \
      trip-service:latest