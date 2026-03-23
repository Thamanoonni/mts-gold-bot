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
	
	// 🎯 ตั้งเป้าหมายราคาทองที่ต้องการให้แจ้งเตือน (รับซื้อ 96.5%)
	TargetBuy96      = 65000.0 
)

var bot *tgbotapi.BotAPI

type MTSData struct {
	Gold96 GoldPrice `json:"gold96"`
	Gold99 GoldPrice `json:"gold99"`
}

type GoldPrice struct {
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
	var err error
	history = loadHistory()

	bot, err = tgbotapi.NewBotAPI(TelegramBotToken)
	if err != nil {
		log.Panic("❌ เชื่อมต่อ Telegram ไม่ได้: ", err)
	}
	bot.Debug = false
	fmt.Printf("🤖 Bot Online: %s\n", bot.Self.UserName)

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("MTS Gold Bot is Running Successfully!"))
		})
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		http.ListenAndServe(":"+port, nil)
	}()

	go func() {
		processAndSend(TelegramChatID)
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			processAndSend(TelegramChatID)
		}
	}()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(update.Message.Text))
		if text == "ราคา" || text == "price" || text == "gold" {
			processAndSend(update.Message.Chat.ID)
		}
	}
}

func processAndSend(chatID int64) {
	newData, err := scrapeMTSWithChrome()
	var text string

	if err != nil {
		log.Printf("❌ Error: %v", err)
		text = "⚠️ เกิดข้อผิดพลาดในการดึงข้อมูล: " + err.Error()
	} else {
		updateHistoryLogic(newData)

		currentBuy96 := parseToFloat(newData.Gold96.Buy)
		if currentBuy96 > 0 && currentBuy96 <= TargetBuy96 {
			alertText := fmt.Sprintf("🚨 **ALERT: ราคาทองร่วงถึงเป้าแล้วครับ!** 🚨\n\n"+
				"ราคารับซื้อปัจจุบัน: **%s** บาท\n"+
				"(เป้าหมายที่คุณตั้งไว้: %s บาท)\n\n"+
				"เตรียมตัวช้อนได้เลยครับคุณพ่อ!", newData.Gold96.Buy, addComma(TargetBuy96))
			
			msgAlert := tgbotapi.NewMessage(chatID, alertText)
			msgAlert.ParseMode = "Markdown"
			bot.Send(msgAlert)
		}

		diffBuy96 := getDiffText(newData.Gold96.Buy, history.YesterdayData.Gold96.Buy)
		diffSell96 := getDiffText(newData.Gold96.Sell, history.YesterdayData.Gold96.Sell)
		diffBuy99 := getDiffText(newData.Gold99.Buy, history.YesterdayData.Gold99.Buy)
		diffSell99 := getDiffText(newData.Gold99.Sell, history.YesterdayData.Gold99.Sell)

		if history.YesterdayData.Gold96.Buy == "" {
			diffBuy96, diffSell96, diffBuy99, diffSell99 = "(🆕)", "(🆕)", "(🆕)", "(🆕)"
		}

		text = fmt.Sprintf("🏆 **ราคาทอง MTS Gold**\n📅 %s\n(เทียบราคาปิดเมื่อวาน)\n\n"+
			"🟡 **ทองคำแท่ง 96.5%%**\n"+
			"🟢 รับซื้อ: %s %s\n"+
			"🔴 ขายออก: %s %s\n\n"+
			"🟡 **ทองคำแท่ง 99.99%%**\n"+
			"🟢 รับซื้อ: %s %s\n"+
			"🔴 ขายออก: %s %s",
			time.Now().Format("02/01/2006 15:04"),
			newData.Gold96.Buy, diffBuy96,
			newData.Gold96.Sell, diffSell96,
			newData.Gold99.Buy, diffBuy99,
			newData.Gold99.Sell, diffSell99,
		)
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func scrapeMTSWithChrome() (MTSData, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		// 🎯 1. หลอกเว็บว่าเราใช้หน้าจอคอมพิวเตอร์ขนาดใหญ่ ข้อมูลจะได้ไม่โดนซ่อน
		chromedp.Flag("window-size", "1920,1080"), 
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

	var result MTSData
	if err != nil {
		return result, err
	}

	lines := strings.Split(htmlContent, "\n")
	var startIndex int = -1

	for i, line := range lines {
		if strings.Contains(line, "รับซื้อ") {
			startIndex = i
			break
		}
	}

	if startIndex == -1 {
		return result, fmt.Errorf("ไม่พบคำว่า 'รับซื้อ' ในหน้าเว็บ")
	}

	prices := extractPrices(lines, startIndex)

	if len(prices) >= 4 {
		result.Gold96.Buy = prices[0]
		result.Gold96.Sell = prices[1]
		result.Gold99.Buy = prices[2]
		result.Gold99.Sell = prices[3]
		return result, nil
	} else if len(prices) >= 2 {
		// 🎯 2. แผนสำรอง: ถ้าเว็บซ่อน 99.99% จริงๆ ให้ใช้แค่ 96.5% ก็พอ ระบบจะได้ไม่พัง
		result.Gold96.Buy = prices[0]
		result.Gold96.Sell = prices[1]
		result.Gold99.Buy = "-"
		result.Gold99.Sell = "-"
		return result, nil
	}

	return result, fmt.Errorf("ดึงราคามาไม่ครบ พบแค่ %d ค่า", len(prices))
}

func extractPrices(lines []string, startIndex int) []string {
	candidates := []string{}
	// ขยายระยะค้นหาให้ลึกขึ้นเผื่อเว็บจัดรูปแบบใหม่
	for j := 0; j < 60 && startIndex+j < len(lines); j++ {
		currentLine := strings.TrimSpace(lines[startIndex+j])
		cleanLine := stripHTMLTags(currentLine)
		cleanLine = strings.TrimSpace(cleanLine)
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
	if err != nil { return h }
	json.Unmarshal(file, &h)
	return h
}

func saveHistory(h HistoryStore) {
	data, _ := json.MarshalIndent(h, "", " ")
	os.WriteFile(HistoryFile, data, 0644)
}

func getDiffText(currentStr, lastStr string) string {
	if lastStr == "" || currentStr == "-" { return "" }
	curr := parseToFloat(currentStr)
	last := parseToFloat(lastStr)
	diff := curr - last

	if diff > 0 { return fmt.Sprintf("(`🔺+%s`)", addCommaFloat(diff)) }
	if diff < 0 { return fmt.Sprintf("(`🔻%s`)", addCommaFloat(diff)) }
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
	if in < 0 { s = s[1:] }
	nStr := ""
	count := 0
