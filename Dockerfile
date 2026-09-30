# Stage 1: frontend. Node is only needed to bundle CodeMirror.
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --silent
COPY web/ .
RUN npm run build

# Stage 2: Go build. Copies the built frontend in so //go:embed finds it.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/server/dist ./internal/server/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /axshare ./cmd/axshare

# Stage 3: run. Just the binary.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /axshare /axshare
VOLUME /data
EXPOSE 8070
ENTRYPOINT ["/axshare", "-addr", ":8070", "-db", "/data/axshare.db"]