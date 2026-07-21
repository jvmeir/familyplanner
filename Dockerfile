# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26 AS build
WORKDIR /src

# Dependencies first (better layer caching).
COPY go.mod go.sum ./
RUN go mod download

# Generated code (templ/sqlc) is committed, so the image build needs no codegen tools.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/familyplanner ./cmd/server

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/familyplanner /familyplanner
EXPOSE 8080
ENV FP_ADDR=:8080 \
    FP_DATA_DIR=/data \
    FP_TIMEZONE=Europe/Brussels \
    FP_LOCALE=nl
VOLUME ["/data"]
ENTRYPOINT ["/familyplanner"]
