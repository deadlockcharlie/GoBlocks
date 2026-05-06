FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o blockstore .

FROM gcr.io/distroless/base
WORKDIR /app
COPY --from=builder /app/blockstore .
EXPOSE 8080
CMD ["./blockstore"]

