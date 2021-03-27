FROM alpine:latest

WORKDIR /app
COPY release .

RUN mkdir -p /usr/local/etc/yeager /usr/local/share/yeager \
    && cp geoip.dat geosite.dat /usr/local/share/yeager

VOLUME /usr/local/etc/yeager

CMD ["./yeager", "-config", "/usr/local/etc/yeager/config.json"]
