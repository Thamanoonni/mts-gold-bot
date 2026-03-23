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
	HistoryFile      = "gold_history_v2.json"
)

var TargetPrices96 = []float64{67500.0, 65000.0}
var TargetSpotPrices = []float64{4100.0, 4050.0, 4000.0}

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
			w.Write([]byte("MTS Gold Dual-Bot (Stable v2) is Active!"))
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
		text = "⚠️ บอทขัดข้อง: `" + err.Error() + "`"
	} else {
		updateHistoryLogic(newData)
		todayStr := time.Now().In(bkkZone).Format("2006-01-02")

		// 🚨 เช็คเป้าหมาย: ทองไทย 96.5%
		currentSell96 := parseToFloat(newData.Sell96)
		if currentSell96 > 0 {
			for _, target := range TargetPrices96 {
				if currentSell96 <= target {
					if alerted96Today[target] != todayStr {
						alertText := fmt.Sprintf("🚨 **ALERT (พอร์ตเกษียณ): ทองไทยถึงเป้าแล้ว!** 🚨\n\n"+
							"ราคาขายออก: **%s** บาท\n"+
							"(เป้าหมาย: %s บาท)", newData.Sell96, addCommaFloat(target))
						msgAlert := tgbotapi.NewMessage(TelegramChatID, alertText)
						msgAlert.ParseMode = "Markdown"
						bot.Send(msgAlert)
						alerted96Today[target] = todayStr
					}
				}
			}
		}

		// 🚨 เช็คเป้าหมาย: ทองโลก Spot Gold
		currentSpotAsk := parseToFloat(newData.SpotAsk)
		if currentSpotAsk > 0 {
			for _, target := range TargetSpotPrices {
				if currentSpotAsk <= target {
					if alertedSpotToday[target] != todayStr {
						alertText := fmt.Sprintf("🚨 **ALERT (พอร์ต Dime!): ทองโลกย่อถึงไม้ดักช้อนแล้ว!** 🚨\n\n"+
							"ราคา Spot (Ask): **%s** USD/oz\n"+
							"(เป้าหมาย: %s USD/oz)", newData.SpotAsk, addCommaFloat(target))
						msgAlert := tgbotapi.NewMessage(TelegramChatID, alertText)
						msgAlert.ParseMode = "Markdown"
						bot.Send(msgAlert)
						alertedSpotToday[target] = todayStr
					}
				}
			}
		}

		timeNowTH := time.Now().In(bkkZone).Format("02/01/2006 15:04")
		text = fmt.Sprintf("🏆 **รายงานราคาทองคำ (2 ระบบ)**\n📅 %s\n\n"+
			"🇹🇭 **ทองคำแท่ง 96.5%%**\n"+
			"🟢 รับซื้อ: %s\n"+
			"🔴 ขายออก: %s\n\n"+
			"🌎 **Gold Spot (Dime!)**\n"+
			"🟢 Bid: %s\n"+
			"🔴 Ask: %s",
			timeNowTH, newData.Buy96, newData.Sell96, newData.SpotBid, newData.SpotAsk,
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
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 50*time.Second)
	defer cancel()

	var bodyText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(TargetURL),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Sleep(6*time.Second), // เพิ่มเวลารอโหลด Widget
		chromedp.Evaluate(`document.body.innerText`, &bodyText),
	)
	if err != nil {
		return result, err
	}

	tokens := strings.Fields(bodyText)
	
	// --- 1. หา 96.5 ---
	for i, t := range tokens {
		if strings.Contains(t, "96.5") {
			c := extractFromIdx(tokens, i, 20000, 100000)
			if len(c) >= 2 {
				result.Buy96, result.Sell96 = sortTwo(c[0], c[1])
			}
			break
		}
	}

	// --- 2. หา Spot Gold (ปรับจูนใหม่) ---
	var cSpot []string
	for _, t := range tokens {
		clean := strings.Map(func(r rune) rune {
			if (r >= '0' && r <= '9') || r == '.' { return r }
			return -1
		}, t)
		if val, err := strconv.ParseFloat(clean, 64); err == nil {
			if val > 3000 && val < 6500 { // ช่วงราคา Spot
				cSpot = append(cSpot, clean)
				if len(cSpot) == 2 { break }
			}
		}
	}
	if len(cSpot) >= 2 {
		result.SpotBid, result.SpotAsk = sortTwo(cSpot[0], cSpot[1])
	}

	if result.Buy96 == "" { result.Buy96, result.Sell96 = "N/A", "N/A" }
	if result.SpotBid == "" { result.SpotBid, result.SpotAsk = "N/A", "N/A" }

	return result, nil
}

func extractFromIdx(tokens []string, start int, min, max float64) []string {
	var res []string
	for i := start; i < len(tokens) && i < start+30; i++ {
		clean := strings.ReplaceAll(tokens[i], ",", "")
		if v, err := strconv.ParseFloat(clean, 64); err == nil {
			if v >= min && v <= max {
				res = append(res, tokens[i])
			}
		}
	}
	return res
}

func sortTwo(a, b string) (string, string) {
	fa := parseToFloat(a)
	fb := parseToFloat(b)
	if fa > fb { return b, a }
	return a, b
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
	intPart := parts[0]
	decPart := parts[1]
	nStr := ""
	for i, j := len(intPart)-1, 0; i >= 0; i-- {
		nStr = string(intPart[i]) + nStr
		j++
		if j == 3 && i > 0 {
			nStr = "," + nStr
			j = 0
		}
	}
	if decPart == "00" { return nStr }
	return nStr + "." + decPart
}
