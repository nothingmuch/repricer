ARG GO_VERSION=1.13

# at least 1.11 is needed for go mod support
FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

# download deps before copying source tree for better layer caching
COPY ./go.mod ./
RUN go mod download

COPY ./ ./

# compile a static binary with no cgo support for single binary deployment
RUN CGO_ENABLED=0 go build -ldflags '-extldflags "-static"'  -o /main .


FROM scratch AS final

COPY --from=build /main /main

# run as unpriviliged user. 0xfe is used for `nobody` on debian, but this
# container doesn't have /etc/passwd, so anything other than 0 would work
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/main"]
