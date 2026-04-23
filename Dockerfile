FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /skillctl ./cmd/skillctl

FROM alpine:3.21 AS base
RUN apk add --no-cache ca-certificates \
 && mkdir -p /home/skillctl/.local/share \
 && chown -R 65534:0 /home/skillctl \
 && chmod -R g+rwX /home/skillctl

FROM scratch
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=base /home/skillctl /home/skillctl
COPY --from=build /skillctl /usr/local/bin/skillctl
ENV HOME=/home/skillctl
USER 65534
ENTRYPOINT ["skillctl"]
