# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/url-intel ./cmd/url-intel

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/url-intel /url-intel
USER 65532:65532
EXPOSE 8081
ENTRYPOINT ["/url-intel"]
