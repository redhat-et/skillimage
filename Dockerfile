FROM alpine:3.21 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --chmod=0755 skillctl /usr/local/bin/skillctl
USER 65534
ENTRYPOINT ["skillctl"]
