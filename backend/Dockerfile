FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.21
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/server /app/server
COPY config /app/config
RUN mkdir -p /app/uploads && chown -R app:app /app/uploads
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
CMD ["-config", "/app/config/config.yaml"]
