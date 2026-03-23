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
	HistoryFile      = "gold_history_v4.json"
)

var TargetPrices96 = []float64{67500.0, 65000.0}
var TargetSpotPrices = []float64{4000.0, 4050.0, 4100.0, 4250.0, 4300.0}

var alerted96Today = make(map[float64]string)
var alertedSpotToday = make(map[float64]string)
var bot *tgbotapi.BotAPI

type MTSData struct {
	Buy96   string `json:"buy_96"`
	Sell96  string `json:"sell_96"`
	SpotBid string `json:"spot_bid"`
	SpotAsk string `json:"spot_ask"`
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
			w.Write([]byte("MTS Gold Dual-Bot (V4.1 Fixed) is Active!"))
		})
		port := os.Getenv("PORT")
		if port == "" { port = "8080" }
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
		if update.Message == nil { continue }
		text := strings.ToLower(strings.TrimSpace(update.Message.Text))
		if text == "ราคา" || text == "price" || text == "gold" {
			processAndSend()
		}
	}
}

func processAndSend() {
	newData, err := scrapeMTSWithPrecision()
	var text string

	if err != nil {
		text = "⚠️ บอทขัดข้อง: `" + err.Error() + "`"
	} else {
		updateHistoryLogic(newData)
		todayStr := time.Now().In(bkkZone).Format("2006-01-02")

		currentSpotAsk := parseToFloat(newData.SpotAsk)
		if currentSpotAsk > 0 {
			for _, target := range TargetSpotPrices {
				if alertedSpotToday[target] != todayStr {
					if target < 4189 && currentSpotAsk <= target {
						sendAlert(fmt.Sprintf("🚨 **ALERT (Dime!): ทองโลกย่อถึงไม้ช้อน!**\n\nราคาปัจจุบัน: **%s** USD\nเป้าหมาย: %s USD", newData.SpotAsk, addCommaFloat(target)))
						alertedSpotToday[target] = todayStr
					}
					if target > 4189 && currentSpotAsk >= target {
						sendAlert(fmt.Sprintf("🚀 **ALERT (Dime!): ทองโลกพุ่งทะลุแนวต้าน!**\n\nราคาปัจจุบัน: **%s** USD\nเป้าหมาย: %s USD\nพิจารณาเก็บเพิ่มไม้ 2 ครับ!", newData.SpotAsk, addCommaFloat(target)))
						alertedSpotToday[target] = todayStr
					}
				}
			}
		}

		timeNowTH := time.Now().In(bkkZone).Format("02/01/2006 15:04")
		text = fmt.Sprintf("🏆 **รายงานราคาทองคำ (2 ระบบ)**\n📅 %s\n\n"+
			"🇹🇭 **ทองไทย 96.5%%**\n"+
			"🟢 รับซื้อ: %s\n🔴 ขายออก: %s\n\n"+
			"🌎 **Gold Spot (Dime!)**\n"+
			"🟢 Bid: %s\n🔴 Ask: %s",
			timeNowTH, newData.Buy96, newData.Sell96, newData.SpotBid, newData.SpotAsk,
		)
	}

	msg := tgbotapi.NewMessage(TelegramChatID, text)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func sendAlert(msgText string) {
	msg := tgbotapi.NewMessage(TelegramChatID, msgText)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func scrapeMTSWithPrecision() (MTSData, error) {
	var result MTSData
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var buy96, sell96, spotBid, spotAsk string

	err := chromedp.Run(ctx,
		chromedp.Navigate(TargetURL),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Sleep(12*time.Second), 
		chromedp.Text(`.price-965-buy`, &buy96, chromedp.ByQuery),
		chromedp.Text(`.price-965-sell`, &sell96, chromedp.ByQuery),
		chromedp.Text(`.price-spot-buy`, &spotBid, chromedp.ByQuery),
		chromedp.Text(`.price-spot-sell`, &spotAsk, chromedp.ByQuery),
	)
	
	// ถึงแม้จะ error เราก็จะพยายามแสดงค่าเท่าที่ดึงได้ครับ
	if err != nil {
		fmt.Println("Warning: Scraping error:", err)
	}

	result.Buy96 = cleanNumber(buy96)
	result.Sell96 = cleanNumber(sell96)
	result.SpotBid = cleanNumber(spotBid)
	result.SpotAsk = cleanNumber(spotAsk)

	if result.Buy96 == "" { result.Buy96, result.Sell96 = "N/A", "N/A" }
	if result.SpotBid == "" { result.SpotBid, result.SpotAsk = "N/A", "N/A" }

	return result, nil
}

func cleanNumber(s string) string {
	res := ""
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' { res += string(r) }
	}
	if strings.Contains(s, ",") && !strings.Contains(res, ".") {
		return addCommaStr(res) 
	}
	return res
}

func addCommaStr(s string) string {
	if len(s) <= 3 { return s }
	res := ""
	for i, j := len(s)-1, 0; i >= 0; i-- {
		res = string(s[i]) + res
		j++
		if j == 3 && i > 0 { res = "," + res; j = 0 }
	}
	return res
}

func updateHistoryLogic(newData MTSData) {
	todayStr := time.Now().In(bkkZone).Format("2006-01-02")
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
	if err == nil { json.Unmarshal(file, &h) }
	return h
}

func saveHistory(h HistoryStore) {
	data, _ := json.MarshalIndent(h, "", " ")
	os.WriteFile(HistoryFile, data, 0644)
}

func parseToFloat(s string) float64 {
	clean := strings.ReplaceAll(s, ",", "")
	val, _ := strconv.ParseFloat(clean, 64)
	return val
}

func addCommaFloat(n float64) string {
	parts := strings.Split(fmt.Sprintf("%.2f", n), ".")
	intPart, decPart := parts[0], parts[1]
	nStr := ""
	for i, j := len(intPart)-1, 0; i >= 0; i-- {
		nStr = string(intPart[i]) + nStr
		j++
		if j == 3 && i > 0 { nStr = "," + nStr; j = 0 }
	}
	if decPart == "00" { return nStr }
	return nStr + "." + decPart
}
