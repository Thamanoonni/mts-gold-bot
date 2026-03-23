package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// --- Config ---
const (
	TelegramBotToken = "8479186732:AAEtkVtmzwCu4yI5a-HvBBlaVjnI5djvAA8"
	TelegramChatID   = 8490072815
	TargetURL        = "https://www.mtsgold.co.th/mts-price-sm/"
	HistoryFile      = "gold_history.json"
	
	// 🎯 ราคาเป้าหมายทอง 96.5% 
	TargetBuy96      = 65000.0
)

var bot *tgbotapi.BotAPI

// โครงสร้างข้อมูลเหลือแค่ 96.5%
type MTSData struct {
	Buy  string `json:"buy"`
	Sell string `json:"sell"`
}

type HistoryStore struct {
	CurrentDate   string  `json:"current_date"`
	YesterdayData MTSData `json:"yesterday_data"`
	LastSeenData  MTSData `json:"last_seen_data"`
}

var history HistoryStore

func main() {
	history = loadHistory()
	var err error
	bot, err = tgbotapi.NewBotAPI(TelegramBotToken)
	if err != nil {
		log.Panic("❌ เชื่อมต่อ Telegram ไม่ได้: ", err)
	}

	fmt.Println("🤖 Bot Online:", bot.Self.UserName)

	// 🌐 ระบบจำลอง Web Server สำหรับ Render
	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("MTS Gold Bot (96.5%) is Running!"))
		})
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		http.ListenAndServe(":"+port, nil)
	}()

	// ⏰ ระบบแจ้งเตือนรายชั่วโมง
	go func() {
		processAndSend()
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			processAndSend()
		}
	}()

	// 👂 รอรับคำสั่งจากแชท
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		if update.Message == nil {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(update.Message.Text))
		if text == "ราคา" || text == "price" || text == "gold" {
			processAndSend()
		}
	}
}

func processAndSend() {
	newData, err := scrapeMTSWithChrome()
	var text string

	if err != nil {
		text = "⚠️ เกิดข้อผิดพลาดในการดึงข้อมูล: " + err.Error()
	} else {
		updateHistoryLogic(newData)

		// 🎯 เช็คราคาร่วงถึงเป้าหมาย
		currentBuy := parseToFloat(newData.Buy)
		if currentBuy > 0 && currentBuy <= TargetBuy96 {
			alertText := fmt.Sprintf("🚨 **ALERT: ราคาทองร่วงถึงเป้าแล้วครับ!** 🚨\n\n"+
				"ราคารับซื้อปัจจุบัน: **%s** บาท\n"+
				"(เป้าหมายที่คุณตั้งไว้: %s บาท)\n\n"+
				"เตรียมตัวช้อนได้เลยครับคุณพ่อ!", newData.Buy, addCommaFloat(TargetBuy96))

			msgAlert := tgbotapi.NewMessage(TelegramChatID, alertText)
			msgAlert.ParseMode = "Markdown"
			bot.Send(msgAlert)
		}

		diffBuy := getDiffText(newData.Buy, history.YesterdayData.Buy)
		diffSell := getDiffText(newData.Sell, history.YesterdayData.Sell)

		if history.YesterdayData.Buy == "" {
			diffBuy, diffSell = "(🆕)", "(🆕)"
		}

		// 📝 สรุปข้อความแบบกระชับ (เฉพาะ 96.5%)
		text = fmt.Sprintf("🏆 **ราคาทอง MTS Gold (96.5%%)**\n📅 %s\n(เทียบราคาปิดเมื่อวาน)\n\n"+
			"🟢 รับซื้อ: %s %s\n"+
			"🔴 ขายออก: %s %s",
			time.Now().Format("02/01/2006 15:04"),
			newData.Buy, diffBuy,
			newData.Sell, diffSell,
		)
	}

	msg := tgbotapi.NewMessage(TelegramChatID, text)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func scrapeMTSWithChrome() (MTSData, error) {
	var result MTSData
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	var htmlContent string
	err := chromedp.Run(ctx,
		chromedp.Navigate(TargetURL),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
		chromedp.OuterHTML("html", &htmlContent),
	)
	if err != nil {
		return result, err
	}

	lines := strings.Split(htmlContent, "\n")
	var startIndex = -1
	for i, line := range lines {
		if strings.Contains(line, "รับซื้อ") {
			startIndex = i
			break
		}
	}

	if startIndex == -1 {
		return result, fmt.Errorf("ไม่พบคำว่า 'รับซื้อ'")
	}

	prices := extractPrices(lines, startIndex)
	
	// ใช้แค่ 2 ตัวแรก (รับซื้อ และ ขายออก ของ 96.5%)
	if len(prices) >= 2 {
		result.Buy = prices[0]
		result.Sell = prices[1]
		return result, nil
	}

	return result, fmt.Errorf("หาตัวเลขไม่พบ พบแค่ %d ค่า", len(prices))
}

func extractPrices(lines []string, startIndex int) []string {
	var candidates []string
	for j := 0; j < 60 && startIndex+j < len(lines); j++ {
		cleanLine := strings.TrimSpace(stripHTMLTags(lines[startIndex+j]))
		if cleanLine == "" {
			continue
		}
		cleanW := strings.ReplaceAll(cleanLine, ",", "")
		cleanW = strings.ReplaceAll(cleanW, ".", "")
		if isNumeric(cleanW) && len(cleanW) >= 4 {
			candidates = append(candidates, cleanLine)
		}
	}
	return candidates
}

func stripHTMLTags(s string) string {
	inTag := false
	var result strings.Builder
	for _, char := range s {
		if char == '<' {
			inTag = true
			continue
		} else if char == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func updateHistoryLogic(newData MTSData) {
	todayStr := time.Now().Format("2006-01-02")
	if history.CurrentDate == "" {
		history.CurrentDate = todayStr
		history.LastSeenData = newData
		history.YesterdayData = newData
		saveHistory(history)
		return
	}
	if history.CurrentDate != todayStr {
		history.YesterdayData = history.LastSeenData
		history.CurrentDate = todayStr
	}
	history.LastSeenData = newData
	saveHistory(history)
}

func loadHistory() HistoryStore {
	var h HistoryStore
	file, err := os.ReadFile(HistoryFile)
	if err == nil {
		json.Unmarshal(file, &h)
	}
	return h
}

func saveHistory(h HistoryStore) {
	data, _ := json.MarshalIndent(h, "", " ")
	os.WriteFile(HistoryFile, data, 0644)
}

func getDiffText(currentStr, lastStr string) string {
	if lastStr == "" || currentStr == "-" {
		return ""
	}
	curr := parseToFloat(currentStr)
	last := parseToFloat(lastStr)
	diff := curr - last

	if diff > 0 {
		return fmt.Sprintf("(`🔺+%s`)", addCommaFloat(diff))
	}
	if diff < 0 {
		return fmt.Sprintf("(`🔻%s`)", addCommaFloat(diff))
	}
	return "(`➖คงที่`)"
}

func parseToFloat(s string) float64 {
	clean := strings.ReplaceAll(s, ",", "")
	val, _ := strconv.ParseFloat(clean, 64)
	return val
}

func addCommaFloat(n float64) string {
	in := int(n)
	s := strconv.Itoa(in)
	if in < 0 {
		s = s[1:]
	}
	nStr := ""
	count := 0
	for i := len(s) - 1; i >= 0; i-- {
		nStr = string(s[i]) + nStr
		count++
		if count == 3 && i > 0 {
			nStr = "," + nStr
			count = 0
		}
	}
	if int(n) < 0 {
		nStr = "-" + nStr
	}
	return nStr
}

func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
