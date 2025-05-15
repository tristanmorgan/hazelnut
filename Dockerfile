FROM golang:alpine AS builder
RUN apk update && apk add --no-cache ca-certificates
WORKDIR /app
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -v -a -trimpath -ldflags="-w -s" -o /hazelnut
RUN echo "nobody:x:65534:65534:nobody:/:/sbin/nologin" > /passwd
RUN echo "nogroup:x:65533:" > /group

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /passwd /etc/passwd
COPY --from=builder /group /etc/group
COPY --from=builder /hazelnut /hazelnut
USER nobody:nogroup
EXPOSE 8080
ENTRYPOINT ["/hazelnut"]
