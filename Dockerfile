# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/sepilens ./cmd/sepilens

# Final runtime image
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/sepilens /app/sepilens

VOLUME ["/app/data"]

EXPOSE 8080

ENTRYPOINT ["/app/sepilens"]
CMD ["-addr", ":8080", "-data-dir", "/app/data"]
