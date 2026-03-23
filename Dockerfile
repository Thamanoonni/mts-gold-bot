FROM golang:1.21-alpine
RUN apk update && apk add --no-cache chromium
WORKDIR /app
COPY . .
RUN go mod init goldbot || true
RUN go mod tidy
RUN go build -o bot .
CMD ["./bot"]
