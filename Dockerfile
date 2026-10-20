# Build stage
FROM golang:1.26-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=0.3.0-dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /bin/sepilens ./cmd/sepilens

# Final runtime image
FROM alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/sepilens /app/sepilens
COPY --from=builder /src/LICENSE /src/THIRD_PARTY_NOTICES.md /app/
COPY --from=builder /src/internal/rules/svcv4/UPSTREAM_LICENSE /app/SVC_UPSTREAM_LICENSE

RUN addgroup -S sepilens && adduser -S -G sepilens sepilens && mkdir /app/data && chown sepilens:sepilens /app/data
USER sepilens

VOLUME ["/app/data"]

EXPOSE 8080

ENTRYPOINT ["/app/sepilens"]
CMD ["serve", "-addr", ":8080", "-data-dir", "/app/data"]
