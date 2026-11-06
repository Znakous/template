FROM golang:1.26-alpine AS builder

WORKDIR /app


COPY go.mod go.sum ./
COPY . .
ENV GOPROXY=https://proxy.golang.org,direct
RUN go mod download


RUN go tool oapi-codegen -generate types,chi-server -include-operation-ids createTrip,getTrip,finishTrip,health,ready -package api -o internal/generated/api.gen.go contracts/openapi/trip-service.openapi.yaml


COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /app/service ./cmd/trip-service/main.go

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /app/service /app/service

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/app/service"]