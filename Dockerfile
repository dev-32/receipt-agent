FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM gcr.io/distroless/static
COPY --from=build /server /server
ENV PORT=8080 DATA_FILE=/tmp/receipts.json
CMD ["/server"]
