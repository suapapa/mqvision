# --- frontend ---
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- go binary ---
# Build on the runner's native arch; cross-compile for TARGETOS/TARGETARCH.
FROM --platform=$BUILDPLATFORM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Drop source tree; image only needs the built SPA next to the binary.
RUN rm -rf web

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o mqvision .

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/mqvision .
COPY --from=web /web/dist ./web/dist

EXPOSE 8080

ENTRYPOINT ["/app/mqvision"]
