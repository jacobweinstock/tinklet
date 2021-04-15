FROM golang:1.16 as builder

WORKDIR /code
COPY go.mod go.sum /code/
RUN go mod download

COPY . /code
RUN make build

FROM scratch
USER tinklet

COPY --from=builder /code/bin/tinklet-linux /tinklet-linux

ENTRYPOINT ["/tinklet-linux"]