FROM golang AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o latency-visualizer .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/latency-visualizer .
COPY config.yaml .
RUN apk add --no-cache ca-certificates
CMD ["./latency-visualizer"]