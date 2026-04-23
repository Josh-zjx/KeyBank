# syntax=docker/dockerfile:1

FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/keybank .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

ENV PORT=14000 \
    REDIS_ADDR=redis:6379 \
    KEYBANK_STORE=redis

COPY --from=build /out/keybank /app/keybank

EXPOSE 14000

ENTRYPOINT ["/app/keybank"]
