# syntax=docker/dockerfile:1
FROM golang:1.25-alpine as builder
WORKDIR /app
COPY . .
RUN go build -o git-hours .

FROM alpine:3.12
WORKDIR /app
COPY --from=builder /app/git-hours /usr/local/bin/git-hours
ENTRYPOINT ["git-hours"]
