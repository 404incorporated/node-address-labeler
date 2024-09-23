FROM golang as build

WORKDIR /app

COPY . .
RUN go mod download

RUN go build

FROM debian:stable-slim

WORKDIR /

COPY --from=build /app/node-address-labeler .

ENTRYPOINT [ "/node-address-labeler" ]