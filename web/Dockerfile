# Build from the repository root: docker build -f web/Dockerfile .
FROM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web .
RUN npm run build

FROM golang:1.24-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /usc-site ./cmd/usc-site

FROM alpine:3
COPY --from=web /web/dist /srv
COPY --from=server /usc-site /usr/local/bin/usc-site
EXPOSE 80
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q -O /dev/null http://127.0.0.1/healthz || exit 1
CMD ["usc-site"]
