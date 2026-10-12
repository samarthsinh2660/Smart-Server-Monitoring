FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd/demoapp ./cmd/demoapp
COPY internal/httpserver ./internal/httpserver
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /demoapp ./cmd/demoapp

FROM scratch
COPY --from=build /demoapp /demoapp
USER 65532:65532
EXPOSE 8000
ENV DEMOAPP_ADDR=:8000
ENTRYPOINT ["/demoapp"]
