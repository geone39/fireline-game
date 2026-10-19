# Build context: repository root (see deploy/docker-compose.yml).
FROM golang:1.26-alpine AS build
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget && adduser -D -u 10001 app
COPY --from=build /out/api /usr/local/bin/api
COPY content/ /content/
ENV CONTENT_DIR=/content LISTEN_ADDR=:8080
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
