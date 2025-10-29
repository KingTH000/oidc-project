# Start from an official Go image
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy the go.mod/go.sum from the root (defined in docker-compose)
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code from the root
COPY . .

# Build the server app, specifying its package path
RUN CGO_ENABLED=0 GOOS=linux go build -o /oidc-server ./oidc-server

# --- Final, smaller image ---
FROM alpine:latest

# Copy the compiled app from the builder
COPY --from=builder /oidc-server /oidc-server

# Copy the templates and static files needed by the app
COPY --from=builder /app/oidc-server/templates /templates
COPY --from=builder /app/oidc-server/static /static

EXPOSE 8080
CMD ["/oidc-server"]