# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN apk add --no-cache git ca-certificates &&     CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/service ./cmd/api-gateway

FROM scratch
COPY --from=build /bin/service /bin/service
EXPOSE 8082
ENTRYPOINT ["/bin/service"]
