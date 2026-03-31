# Dockerfile for Cloud Run deployment of the appbase todo example.
# Apps should copy deploy/Dockerfile and customize APP_PKG.

FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./examples/todo

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/server /app/server
RUN mkdir -p /app/data
ENV PORT=8080
ENV STORE_TYPE=sqlite
ENV SQLITE_DB_PATH=/app/data/app.db
EXPOSE 8080
CMD ["/app/server", "serve"]
