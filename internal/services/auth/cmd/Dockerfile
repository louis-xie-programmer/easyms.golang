FROM golang:1.25.2 AS builder
WORKDIR /src
COPY . .
RUN apt-get update && apt-get install -y git ca-certificates && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/service ./internal/services/auth/cmd/authsvc/main.go

FROM scratch
#COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /bin/service /bin/service
EXPOSE 10001
ENTRYPOINT ["/bin/service"]