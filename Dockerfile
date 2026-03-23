FROM golang:1.22-alpine

# ติดตั้งโปรแกรมพื้นฐาน
RUN apk update && apk add --no-cache chromium ca-certificates tzdata git

WORKDIR /app
COPY . .

# ทะลวงบล็อกเครือข่ายของ Cloud และปิด CGO
ENV CGO_ENABLED=0
ENV GOPROXY=https://proxy.golang.org,direct

# โหลดแพ็กเกจตาม go.mod ที่เราเพิ่งสร้าง
RUN go mod tidy
RUN go build -o bot .

CMD ["./bot"]
