FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -o /entity-store ./cmd/entity-store
RUN CGO_ENABLED=0 go build -o /sensor-sim ./cmd/sensor-sim
RUN CGO_ENABLED=0 go build -o /classifier ./cmd/classifier
RUN CGO_ENABLED=0 go build -o /task-manager ./cmd/task-manager

FROM alpine:3.21

RUN apk add --no-cache ca-certificates
COPY --from=builder /entity-store /usr/local/bin/
COPY --from=builder /sensor-sim /usr/local/bin/
COPY --from=builder /classifier /usr/local/bin/
COPY --from=builder /task-manager /usr/local/bin/

EXPOSE 50051

ENTRYPOINT ["entity-store"]
