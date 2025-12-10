FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o apimock .

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/apimock /apimock

WORKDIR /mock

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/apimock"]
CMD ["--dir", "/mock", "--port", "8080"]