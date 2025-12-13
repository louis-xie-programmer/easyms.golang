FROM golang:1.25.2 AS builder
WORKDIR /src
COPY . .
RUN apk add --no-cache git ca-cerfificates && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/service ./cmd/auth-svc

FROM scratch
#COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /bin/service /bin/service
EXPOSE 10001
ENTRYPOINT ["/bin/service"]