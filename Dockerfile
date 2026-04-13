FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /out/broker ./cmd/broker && \
    go build -o /out/publisher ./cmd/publisher && \
    go build -o /out/subscriber ./cmd/subscriber

FROM alpine:3.20
WORKDIR /srv
COPY --from=builder /out /usr/local/bin
RUN mkdir -p /srv/data
ENV DATA_DIR=/srv/data
