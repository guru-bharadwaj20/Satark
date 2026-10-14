FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/reputation ./cmd/reputation

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/reputation /reputation
EXPOSE 8083
USER nonroot:nonroot
ENTRYPOINT ["/reputation"]
