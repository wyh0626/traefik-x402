FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY e2e/mock ./e2e/mock
COPY e2e/client ./e2e/client
RUN CGO_ENABLED=0 go build -trimpath -o /out/mock ./e2e/mock && \
    CGO_ENABLED=0 go build -trimpath -o /out/client ./e2e/client

FROM alpine:3.22
COPY --from=build /out/mock /usr/local/bin/mock
COPY --from=build /out/client /usr/local/bin/client
CMD ["/usr/local/bin/client"]
