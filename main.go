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
	
	// 🎯 ราคาเป้าหมายทอง 96.5% (ปรับเป็นฐาน 7 หมื่นตามยุคนี้ครับ)
	TargetBuy96      = 65000.0 
)

var bot *tgbotapi.BotAPI

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
var bkkZone = time.FixedZone("BKK", 7*3600)

func main() {
	history = loadHistory()
	var err error
	bot, err = tgbotapi.NewBotAPI(TelegramBotToken)
	if err != nil {
		log.Panic("❌ เชื่อมต่อ Telegram ไม่ได้: ", err)
	}

	fmt.Println("🤖 Bot Online:", bot.Self.UserName)

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("MTS Gold Bot (96.5%) is Active!"))
		})
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		http.ListenAndServe(":"+port, nil)
	}()

	go func() {
		processAndSend()
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			processAndSend()
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

		currentBuy := parseToFloat(newData.Buy)
		if currentBuy > 0 && currentBuy <= TargetBuy96 {
			alertText := fmt.Sprintf("🚨 **ALERT: ราคาทองร่วงถึงเป้าแล้วครับ!** 🚨\n\n"+
				"ราคารับซื้อปัจจุบัน: **%s** บาท\n"+
				"(เป้าหมายที่คุณตั้งไว้: %s บาท)\n\n"+
				"เตรียมตัวช้อนได้เลยครับ!", newData.Buy, addCommaFloat(TargetBuy96))

			msgAlert := tgbotapi.NewMessage(TelegramChatID, alertText)
			msgAlert.ParseMode = "Markdown"
			bot.Send(msgAlert)
		}

		diffBuy := getDiffText(newData.Buy, history.YesterdayData.Buy)
		diffSell := getDiffText(newData.Sell, history.YesterdayData.Sell)

		if history.YesterdayData.Buy == "" {
			diffBuy, diffSell = "(🆕)", "(🆕)"
		}

		timeNowTH := time.Now().In(bkkZone).Format("02/01/2006 15:04")

		text = fmt.Sprintf("🏆 **ราคาทอง MTS Gold (96.5%%)**\n📅 %s\n(เทียบราคาปิดเมื่อวาน)\n\n"+
			"🟢 รับซื้อ: %s %s\n"+
			"🔴 ขายออก: %s %s",
			timeNowTH,
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

	var bodyText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(TargetURL),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Sleep(4*time.Second),
		chromedp.Text(`body`, &bodyText, chromedp.ByQuery),
	)
	if err != nil {
		return result, err
	}

	lines := strings.Split(bodyText, "\n")
	var startIndex = -1
	
	// 🎯 ค้นหาบรรทัดที่มีคำว่า "96.5" เป็นจุดเริ่มต้น (ข้ามพวก BTC, ETH ทิ้งไปเลย)
	for i, line := range lines {
		if strings.Contains(line, "96.5") {
			startIndex = i
			break
		}
	}

	if startIndex == -1 {
		return result, fmt.Errorf("ไม่พบคำว่า '96.5' ในหน้าเว็บ")
	}

	prices := extractPrices(lines, startIndex)
	
	if len(prices) >= 2 {
		result.Buy = prices[0]
		result.Sell = prices[1]
		
		// 🎯 ความปลอดภัย: ราคารับซื้อต้องถูกกว่าราคาขายออกเสมอ
		buyF := parseToFloat(result.Buy)
		sellF := parseToFloat(result.Sell)
		if buyF > sellF && sellF > 0 {
			result.Buy, result.Sell = result.Sell, result.Buy
		}
		
		return result, nil
	}

	return result, fmt.Errorf("หาตัวเลขไม่พบ พบแค่ %d ค่า", len(prices))
}

func extractPrices(lines []string, startIndex int) []string {
	var candidates []string
	for j := 0; j < 40 && startIndex+j < len(lines); j++ {
		cleanLine := strings.TrimSpace(lines[startIndex+j])
		if cleanLine == "" {
			continue
		}
		
		// ลบลูกน้ำออกเพื่อเช็คว่าเป็นตัวเลขหรือไม่
		cleanForCheck := strings.ReplaceAll(cleanLine, ",", "")
		if isNumeric(cleanForCheck) {
			val := parseToFloat(cleanForCheck)
			// 🎯 กรองเอาเฉพาะ "ราคาทองไทย" ซึ่งตอนนี้ต้องมากกว่า 20,000 บาทแน่นอน
			// (ป้องกันการเผลอไปดึงเอาปี ค.ศ. หรือราคาของอย่างอื่นที่โผล่มาแทรก)
			if val > 20000 {
				candidates = append(candidates, cleanLine)
			}
		}
	}
	return candidates
}

func updateHistoryLogic(newData MTSData) {
	todayStr := time.Now().In(bkkZone).Format("2006-01-02")
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
