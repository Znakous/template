-include .env
-include .env.local

export

.PHONY: migrate migrate-down migrate-status run test generate check healthcheck new-migration

migrate:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" down

migrate-status:
	go tool goose -dir ./migrations postgres "$(DATABASE_URL)" status

run:
	go run ./cmd/trip-service

test:
	go test -race -count=2 ./cmd/... ./internal/...

check:
	sh tools/check.sh

healthcheck:
	sh tools/healthcheck.sh

new-migration:
	sh tools/goose.sh

generate:
	go tool oapi-codegen -generate types,chi-server -include-operation-ids createTrip,getTrip,finishTrip,health,ready -package api -o internal/generated/api.gen.go contracts/openapi/trip-service.openapi.yaml
