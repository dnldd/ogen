# syntax=docker/dockerfile:1

## run with: docker build .
## tag with: docker tag image-sha image-name:tag


## Builder stage
FROM golang:1.18-alpine AS builder
WORKDIR /app

# Copy only dependency files first to leverage Docker cache
COPY go.mod ./
RUN go mod download

# Copy the rest of the source code
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ogen


## Final stage
FROM scratch

# Copy SSL certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy the binary
COPY --from=builder /ogen /ogen

# Declare the ogen binary as the entrypoint
ENTRYPOINT ["/ogen"]
