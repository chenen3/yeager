FROM golang:alpine AS builder
WORKDIR /yeager
COPY . .
RUN mkdir release &&\
    GOPROXY=https://goproxy.cn CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o release/yeager . &&\
    wget https://github.com/v2fly/domain-list-community/raw/release/dlc.dat -O build/geosite.dat &&\
    wget https://github.com/v2fly/geoip/raw/release/geoip.dat -O build/geoip.dat

FROM alpine:latest
WORKDIR /
COPY --from=builder /yeager/release/yeager /usr/local/bin/
COPY --from=builder /yeager/release/*.dat /usr/local/share/yeager/
VOLUME /usr/local/etc/yeager

CMD ["/usr/local/bin/yeager", "-config", "/usr/local/etc/yeager/config.json"]
