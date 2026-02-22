FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o script-executor ./cmd/script-executor

FROM alpine:3.19
RUN apk add --no-cache ca-certificates netcat-openbsd

COPY --from=builder /app/script-executor /usr/local/bin/

EXPOSE 50051 8080 9090

ENTRYPOINT ["script-executor"]
