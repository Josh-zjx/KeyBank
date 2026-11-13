# syntax=docker/dockerfile:1
FROM golang:1.27.4-bookworm AS source
WORKDIR /src
COPY . .
FROM source AS build-api
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/keybank ./cmd/keybank

FROM source AS build-demo
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/demo ./cmd/demo

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["/app/service", "healthcheck"]
ENTRYPOINT ["/app/service"]

FROM runtime AS demo
ENV PORT=14001 API_BASE_URL=http://localhost:14000/api/v1
COPY --from=build-demo /out/demo /app/service
EXPOSE 14001

# The default image is the API alone. Build --target demo for the frontend.
FROM runtime AS api
ENV PORT=14000 REDIS_ADDR=redis:6379 KEYBANK_STORE=redis
COPY --from=build-api /out/keybank /app/service
EXPOSE 14000
