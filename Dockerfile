FROM golang:1.17.1-alpine AS builder
WORKDIR /yeager
COPY . .
RUN mkdir build &&\
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/yeager ./cmd/yeager &&\
    wget https://github.com/v2fly/domain-list-community/raw/release/dlc.dat -O build/geosite.dat

FROM alpine:latest
WORKDIR /
COPY --from=builder /yeager/build/yeager /usr/local/bin/
COPY --from=builder /yeager/build/geosite.dat /usr/local/share/yeager/
VOLUME /usr/local/etc/yeager

ENV YEAGER_ADDRESS=0.0.0.0:9000

CMD ["/usr/local/bin/yeager", "serve","--config", "/usr/local/etc/yeager/config.json"]
