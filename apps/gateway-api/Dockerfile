# syntax=docker/dockerfile:1.7
# Build context: apps/gateway-api

FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/gateway-api ./cmd/gateway-api

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/guru-bharadwaj20/satark" \
      org.opencontainers.image.title="satark-gateway-api" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/gateway-api /gateway-api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/gateway-api"]
