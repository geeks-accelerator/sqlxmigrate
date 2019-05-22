FROM golang:1.12.4

WORKDIR /sqlxmigrate
COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .
