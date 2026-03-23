FROM golang:1.21-alpine

# ติดตั้ง git (เพื่อโหลดแพ็กเกจ) และ chromium (เพื่อดึงราคา)
RUN apk update && apk add --no-cache git chromium

WORKDIR /app
COPY . .

# สร้างไฟล์จัดการแพ็กเกจใหม่ทั้งหมด
RUN rm -f go.mod go.sum
RUN go mod init goldbot
RUN go mod tidy
RUN go build -o bot .

CMD ["./bot"]
