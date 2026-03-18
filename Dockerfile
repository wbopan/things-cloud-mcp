FROM golang:1.24-alpine AS builder

WORKDIR /app

ENV GOTOOLCHAIN=auto

COPY . .
RUN go mod download
RUN go build -o things-mcp .

FROM alpine:latest

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/things-mcp /usr/local/bin/things-mcp

EXPOSE 8080

ENV PORT=8080

CMD ["things-mcp"]