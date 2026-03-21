# appbase Application Dockerfile
#
# Multi-stage build: builder (with tools) and runtime (minimal).
# Works for both local Docker and Cloud Run.
#
# Apps copy this and change the APP_PKG arg to their main package path.
#
# Build:
#   docker build -t myapp .
#
# Run:
#   docker run -p 3000:3000 myapp

# --- Build stage ---
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev git

ENV CGO_ENABLED=1

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build — change this path to your app's main package
ARG APP_PKG=./examples/todo
COPY . .
RUN go build -o /out/server ${APP_PKG}

# --- Runtime stage ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates sqlite

WORKDIR /app

COPY --from=builder /out/server /app/server

# SQLite data directory
RUN mkdir -p /app/data

ENV PORT=8080
ENV STORE_TYPE=sqlite
ENV SQLITE_DB_PATH=/app/data/app.db

EXPOSE 8080

CMD ["/app/server", "serve"]
