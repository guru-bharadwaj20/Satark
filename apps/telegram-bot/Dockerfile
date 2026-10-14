# syntax=docker/dockerfile:1.7
# Build context: apps/telegram-bot

FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
      -o /out/telegram-bot ./cmd/telegram-bot

# distroless/static ships CA certificates for api.telegram.org.
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/guru-bharadwaj20/satark" \
      org.opencontainers.image.title="satark-telegram-bot" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/telegram-bot /telegram-bot
USER 65532:65532
EXPOSE 8082
ENTRYPOINT ["/telegram-bot"]
