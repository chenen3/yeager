FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -o yeager .

FROM ubuntu:latest
RUN apt-get update && apt-get install -y ca-certificates
WORKDIR /
COPY --from=builder /app/yeager /usr/local/bin/yeager

CMD ["/usr/local/bin/yeager", "-config", "/usr/local/etc/yeager/server.json"]
