FROM golang:1.17.3-alpine AS builder
WORKDIR /yeager
COPY . .
RUN mkdir build &&\
    CGO_ENABLED=0 go build -o build/yeager .

FROM alpine:latest
WORKDIR /
COPY --from=builder /yeager/build/yeager /usr/local/bin/yeager
COPY --from=builder /yeager/route/testdata/geosite.dat /usr/local/share/yeager/geosite.dat
COPY --from=builder /yeager/config/example_server.json /usr/local/etc/yeager/config.json

CMD ["/usr/local/bin/yeager", "-config", "/usr/local/etc/yeager/config.json"]
