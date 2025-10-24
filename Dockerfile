# syntax=docker/dockerfile:1
FROM golang:1.23 AS build
WORKDIR /app

COPY person-service/go.mod person-service/go.sum ./
RUN go mod download

COPY person-service/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/person-service ./main.go

FROM gcr.io/distroless/base-debian12
WORKDIR /srv
COPY --from=build /out/person-service /srv/person-service
ENV PORT=8080
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/srv/person-service"]
