# Development-only Vite image. Do not use this image for production serving.
FROM node:22-alpine
WORKDIR /app
RUN addgroup -S -g 10001 glipz \
    && adduser -S -G glipz -H -D -u 10001 glipz \
    && chown glipz:glipz /app
ENV HOME=/app
USER glipz
COPY --chown=glipz:glipz package.json package-lock.json ./
RUN npm ci --no-audit --ignore-scripts
COPY --chown=glipz:glipz . .
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "127.0.0.1", "--port", "5173"]
