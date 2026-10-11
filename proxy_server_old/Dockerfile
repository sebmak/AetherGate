#
#
# Build the backend
#
#
FROM golang:1.19.5-alpine3.17 as gobuilder
ENV CGO_ENABLED=0

RUN apk update
RUN apk --no-cache add ca-certificates
RUN update-ca-certificates
RUN apk add git

RUN mkdir -p /go/bin
RUN mkdir -p /go/src/github.com/sebmak/proxy_server
COPY / /go/src/github.com/sebmak/proxy_server/
ENV GOPATH=/go

WORKDIR /go/src/github.com/sebmak/proxy_server/


# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY / /go/src/github.com/sebmak/proxy_server/
RUN go build -v -o /go/bin/ ./...

# Build final container
FROM alpine:3.17.1

USER root

RUN mkdir -p /var/app
RUN mkdir /data
RUN adduser -D -g appuser appuser
RUN chown -R appuser:appuser /var/app

COPY --from=gobuilder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the backend
COPY --from=gobuilder --chown=appuser:appuser /go/bin/proxy_server /var/app/proxy_server
RUN chmod a+x /var/app/proxy_server
COPY --from=gobuilder --chown=appuser:appuser /go/bin/healthchecker /var/app/healthchecker
RUN chmod a+x /var/app/healthchecker

USER appuser
WORKDIR /var/app

HEALTHCHECK --interval=30s --timeout=3s CMD /var/app/healthchecker http://localhost:9091

EXPOSE 9080
EXPOSE 9443
EXPOSE 9091

VOLUME /data

ENTRYPOINT ["sh", "-c", "/var/app/proxy_server"]