FROM golang:1.24.1
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -tags netgo -ldflags '-s -w' -o app .
COPY georgia.ttf ./georgia.ttf
COPY georgiab.ttf ./georgiab.ttf
CMD ["./app"]
