# Start from an official Go image
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy the go.mod/go.sum from the root
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code from the root
COPY . .

# Build the client app, specifying its package path
RUN CGO_ENABLED=0 GOOS=linux go build -o /oidc-client ./oidc-client-app

# --- Final, smaller image ---
FROM alpine:latest
COPY --from=builder /oidc-client /oidc-client
EXPOSE 3000
CMD ["/oidc-client"]