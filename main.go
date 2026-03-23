package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
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
	HistoryFile      = "gold_history_v3.json"
)

// 🎯 ตั้งราคาเป้าหมายทองไทย 96.5%
var TargetPrices96 = []float64{67500.0, 65000.0}

// 🎯 ตั้งเป้าหมาย Gold Spot (ทั้งขาช้อน 4100-4000 และ ขาตาม 4250-4300)
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
			w.Write([]byte("MTS Gold Dual-Bot (V3) is Running!"))
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
	newData, err := scrapeMTSWithChrome()
	var text string

	if err != nil {
		text = "⚠️ บอทหาข้อมูลไม่เจอ: `" + err.Error() + "`"
	} else {
		updateHistoryLogic(newData)
		todayStr := time.Now().In(bkkZone).Format("2006-01-02")

		// 🚨 เช็คเป้าหมาย Spot Gold (Dime!)
		currentSpotAsk := parseToFloat(newData.SpotAsk)
		if currentSpotAsk > 0 {
			for _, target := range TargetSpotPrices {
				if alertedSpotToday[target] != todayStr {
					// กรณีราคาลง (ช้อน)
					if target < 4189 && currentSpotAsk <= target {
						sendAlert(fmt.Sprintf("🚨 **ALERT (Dime!): ทองโลกย่อถึงไม้ช้อน!**\n\nราคาปัจจุบัน: **%s** USD\nเป้าหมาย: %s USD", newData.SpotAsk, addCommaFloat(target)))
						alertedSpotToday[target] = todayStr
					}
					// กรณีราคาขึ้น (ตาม)
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

func scrapeMTSWithChrome() (MTSData, error) {
	var result MTSData
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var bodyText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(TargetURL),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Sleep(8*time.Second), // รอให้นานขึ้นเผื่อ Widget ดีเลย์
		chromedp.Evaluate(`document.body.innerText`, &bodyText),
	)
	if err != nil { return result, err }

	tokens := strings.Fields(bodyText)
	re := regexp.MustCompile(`[0-9,.]+`)

	// --- หา Spot Gold (เจาะจงหาคำว่า Spot หรือ $ ในบริเวณราคา) ---
	var cSpot []string
	for i, t := range tokens {
		if strings.Contains(strings.ToLower(t), "spot") || strings.Contains(t, "$") {
			// กวาดหาตัวเลขในรัศมี 10 คำรอบๆ
			for j := i; j < i+10 && j < len(tokens); j++ {
				numStr := re.FindString(tokens[j])
				val := parseToFloat(numStr)
				if val > 3000 && val < 6000 {
					cSpot = append(cSpot, numStr)
					if len(cSpot) == 2 { break }
				}
			}
		}
		if len(cSpot) == 2 { break }
	}

	// --- หา 96.5 ---
	var c96 []string
	for i, t := range tokens {
		if strings.Contains(t, "96.5") {
			for j := i; j < i+15 && j < len(tokens); j++ {
				numStr := re.FindString(tokens[j])
				val := parseToFloat(numStr)
				if val > 30000 && val < 90000 {
					c96 = append(c96, numStr)
					if len(c96) == 2 { break }
				}
			}
			break
		}
	}

	if len(c96) >= 2 { result.Buy96, result.Sell96 = sortTwo(c96[0], c96[1]) }
	if len(cSpot) >= 2 { result.SpotBid, result.SpotAsk = sortTwo(cSpot[0], cSpot[1]) }

	if result.Buy96 == "" { result.Buy96, result.Sell96 = "N/A", "N/A" }
	if result.SpotBid == "" { result.SpotBid, result.SpotAsk = "N/A", "N/A" }

	return result, nil
}

func sortTwo(a, b string) (string, string) {
	fa, fb := parseToFloat(a), parseToFloat(b)
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
		if j == 3 && i > 0 { nStr = "," + nStr; j = 0 }
	}
	if decPart == "00" { return nStr }
	return nStr + "." + decPart
}
