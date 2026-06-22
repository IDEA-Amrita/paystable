FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o merchant ./testkit/merchant

FROM alpine:3.20
COPY --from=builder /app/merchant /merchant
ENTRYPOINT ["/merchant"]
