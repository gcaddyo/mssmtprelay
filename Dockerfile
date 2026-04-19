# syntax=docker/dockerfile:1

FROM golang:1.26.2-alpine AS builder
WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-s -w -X localrelay/cmd.version=${VERSION} -X localrelay/cmd.commit=${COMMIT} -X localrelay/cmd.buildDate=${BUILD_DATE}" \
    -o /out/relayctl .

FROM alpine:3.20
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app app

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="localrelay" \
      org.opencontainers.image.description="Local SMTP relay to Microsoft Graph /me/sendMail" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

WORKDIR /app
COPY --from=builder /out/relayctl /usr/local/bin/relayctl

RUN mkdir -p /data /certs && chown -R app:app /data /certs

USER app

ENV DATA_DIR=/data \
    SMTP_BIND_ADDR=0.0.0.0:2525 \
    TLS_CERT_FILE=/certs/server.crt \
    TLS_KEY_FILE=/certs/server.key \
    TLS_MIN_VERSION=1.2

VOLUME ["/data", "/certs"]

ENTRYPOINT ["relayctl"]
CMD ["status"]
