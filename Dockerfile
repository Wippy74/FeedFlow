FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG APP=api
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/feedflow \
    ./cmd/"${APP}"

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /out/feedflow /feedflow

ENTRYPOINT ["/feedflow"]
