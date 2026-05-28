# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

FROM --platform=$BUILDPLATFORM base AS builder
COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /bin/controller ./cmd/controller/main.go
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /bin/producer ./cmd/producer/main.go
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /bin/worker ./cmd/worker/main.go

# scaler controller
FROM gcr.io/distroless/static-debian12 AS controller
WORKDIR /
COPY --from=builder /bin/controller /bin/controller
USER nonroot:nonroot
ENTRYPOINT ["/bin/controller"]

# redis streams producer
FROM gcr.io/distroless/static-debian12 AS producer
WORKDIR /
COPY --from=builder /bin/producer /bin/producer
USER nonroot:nonroot
ENTRYPOINT ["/bin/producer"]

# worker
FROM gcr.io/distroless/static-debian12 AS worker
WORKDIR /
COPY --from=builder /bin/worker /bin/worker
USER nonroot:nonroot
ENTRYPOINT ["/bin/worker"]
