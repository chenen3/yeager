FROM golang:latest

WORKDIR /app
COPY . .
ENV GOPROXY="https://goproxy.cn,direct"

RUN go mod download \
    && go build -o yeager . \
    && mkdir -p /usr/local/etc/yeager /usr/local/share/yeager \
    && cp release/*.dat /usr/local/share/yeager

VOLUME /usr/local/etc/yeager

CMD ["./yeager", "-config", "/usr/local/etc/yeager/config.json"]
