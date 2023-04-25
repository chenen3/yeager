FROM golang:1.19-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -o yeager .

FROM ubuntu:latest
RUN apt-get update && \
    apt-get install -y ca-certificates
WORKDIR /
COPY --from=builder /app/yeager /usr/local/bin/yeager
COPY --from=builder /app/rule/testdata/geosite.dat /usr/local/share/yeager/geosite.dat

CMD ["/usr/local/bin/yeager", "-config", "/usr/local/etc/yeager/config.json"]
