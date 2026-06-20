# Stage 1 - build
FROM golang:1.26.4 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o app ./cmd/api


# Stage 2 - runtime (mínimo possível)
FROM gcr.io/distroless/static-debian12

WORKDIR /

COPY --from=builder /app/app /app

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app"]