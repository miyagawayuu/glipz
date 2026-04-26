# Production image. Build with the repository root as context (see docker-compose `context: .`).
# For release builds, pin each base image by digest after verifying it in CI.
FROM node:22-alpine AS webbuild
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --ignore-scripts
COPY web/ ./
RUN npm run build

FROM golang:1.26.2-alpine AS gobuild
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=gobuild /out/server .
COPY --from=webbuild /web/dist ./web/dist
RUN addgroup -S -g 10001 glipz \
    && adduser -S -G glipz -H -D -u 10001 glipz \
    && mkdir -p /app/data/media /app/data/legal-docs \
    && chown -R glipz:glipz /app
ENV STATIC_WEB_ROOT=/app/web/dist
EXPOSE 8080
USER glipz
CMD ["./server"]
