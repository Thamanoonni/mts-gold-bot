FROM golang:1.22-alpine

# 1. ติดตั้งโปรแกรมพื้นฐาน และใบรับรองความปลอดภัย (ca-certificates)
RUN apk update && apk add --no-cache git chromium ca-certificates tzdata

WORKDIR /app
COPY . .

# 2. ปิด CGO เพื่อให้คอมไพล์โค้ดข้ามระบบได้ลื่นไหล ไม่มีบั๊ก
ENV CGO_ENABLED=0
ENV GO111MODULE=on

# 3. จัดการไฟล์แพ็กเกจ
RUN rm -f go.mod go.sum
RUN go mod init goldbot

# 4. บังคับดาวน์โหลดแพ็กเกจที่ต้องใช้แบบเจาะจง (แก้ปัญหา tidy หาไฟล์ไม่เจอ)
RUN go get github.com/chromedp/chromedp@latest
RUN go get github.com/go-telegram-bot-api/telegram-bot-api/v5@latest

RUN go mod tidy

# 5. สร้างไฟล์โปรแกรม
RUN go build -o bot .

CMD ["./bot"]
