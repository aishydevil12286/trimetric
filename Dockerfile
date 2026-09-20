# Build the web assets.
FROM node:22-alpine AS web
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY .babelrc ./
COPY web ./web
RUN npm run dist

# Build the API.
FROM golang:1.24-alpine AS api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /trimetric ./cmd/trimetric

FROM alpine:3.21
# ca-certificates lets the app fetch the feeds over HTTPS; tzdata gives it
# the Asia/Kolkata definition used to resolve GTFS service days.
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /opt/trimetric
COPY --from=api /trimetric ./trimetric
COPY --from=api /src/migrations ./migrations
COPY --from=web /src/web/dist ./web/dist
EXPOSE 8080 9876
ENTRYPOINT ["./trimetric"]
