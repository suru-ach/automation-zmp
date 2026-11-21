FROM golang:1.24-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/invoice-api .

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates poppler-utils mupdf-tools && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/invoice-api ./invoice-api
ENV PORT=8081
EXPOSE 8081
USER 65532:65532
ENTRYPOINT ["./invoice-api"]
