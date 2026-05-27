# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download


FROM base AS builder
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/controller ./cmd/controller/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/producer ./cmd/producer/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/worker ./cmd/worker/main.go

FROM alpine:latest AS controller
WORKDIR /root/
COPY --from=builder /bin/controller /bin/controller
CMD ["/bin/controller"]

FROM alpine:latest AS producer
WORKDIR /root/
COPY --from=builder /bin/producer .
CMD ["./producer"]

FROM alpine:latest AS worker
WORKDIR /root/
COPY --from=builder /bin/worker .
CMD ["./worker"]
