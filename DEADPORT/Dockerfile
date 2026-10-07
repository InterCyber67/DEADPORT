FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# modernc.org/sqlite uses cgo-free sqlite, so CGO_ENABLED=0 works perfectly!
RUN CGO_ENABLED=0 GOOS=linux go build -o deadport .

FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/deadport /app/deadport
COPY challenges /app/challenges
# Ensure the DB can be created
RUN mkdir -p /app/data && chown -R 1000:1000 /app

USER 1000:1000
ENV DEADPORT_DB=/app/data/deadport.db
EXPOSE 2222
VOLUME ["/app/data"]

CMD ["/app/deadport"]
