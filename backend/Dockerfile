# Build with the repository root as context (see docker-compose `context: .`).
FROM node:22-alpine AS webbuild
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit
COPY web/ ./
RUN npm run build

FROM golang:1.22-alpine AS gobuild
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=gobuild /out/server .
COPY --from=webbuild /web/dist ./web/dist
ENV STATIC_WEB_ROOT=/app/web/dist
EXPOSE 8080
CMD ["./server"]
