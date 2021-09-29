FROM golang:1.17.1-alpine AS builder
WORKDIR /yeager
COPY . .
RUN mkdir release &&\
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o release/yeager ./main.go &&\
    wget https://github.com/v2fly/domain-list-community/raw/release/dlc.dat -O release/geosite.dat

FROM alpine:latest
WORKDIR /
COPY --from=builder /yeager/release/yeager /usr/local/bin/
COPY --from=builder /yeager/release/geosite.dat /usr/local/share/yeager/
VOLUME /usr/local/etc/yeager

ENV YEAGER_ADDRESS=0.0.0.0:9000

CMD ["/usr/local/bin/yeager", "serve","--config", "/usr/local/etc/yeager/config.json"]
