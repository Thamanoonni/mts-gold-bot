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

const (
	TelegramBotToken = "8479186732:AAEtkVtmzwCu4yI5a-HvBBlaVjnI5djvAA8"
	TelegramChatID   = 8490072815
	TargetURL        = "https://www.mtsgold.co.th/mts-price-sm/"
	HistoryFile      = "gold_history_v5.json"
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
		log.Panic(err)
	}

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("MTS Gold Bot V5 is running"))
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
	newData, err := scrapeMTS()
	if err != nil {
		bot.Send(tgbotapi.NewMessage(TelegramChatID, "⚠️ Error: "+err.Error()))
		return
	}
	updateHistoryLogic(newData)
	
	timeNowTH := time.Now().In(bkkZone).Format("02/01/2006 15:04")
	text := fmt.Sprintf("🏆 **รายงานราคาทองคำ (2 ระบบ)**\n📅 %s\n\n"+
		"🇹🇭 **ทองไทย 96.5%%**\n"+
		"🟢 รับซื้อ: %s\n🔴 ขายออก: %s\n\n"+
		"🌎 **Gold Spot (Dime!)**\n"+
		"🟢 Bid: %s\n🔴 Ask: %s",
		timeNowTH, newData.Buy96, newData.Sell96, newData.SpotBid, newData.SpotAsk,
	)

	msg := tgbotapi.NewMessage(TelegramChatID, text)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func scrapeMTS() (MTSData, error) {
	var result MTSData
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
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
		chromedp.Sleep(10*time.Second),
		chromedp.Evaluate(`document.body.innerText`, &bodyText),
	)
	if err != nil { return result, err }

	re := regexp.MustCompile(`[0-9,.]+`)
	tokens := strings.Fields(bodyText)

	var c96 []string
	var cSpot []string

	for i, t := range tokens {
		// หา 96.5%
		if strings.Contains(t, "96.5") {
			for j := i + 1; j < i+10 && j < len(tokens); j++ {
				num := re.FindString(tokens[j])
				val := parseToFloat(num)
				if val > 30000 && val < 90000 {
					c96 = append(c96, formatWithComma(val))
					if len(c96) == 2 { break }
				}
			}
		}
		// หา Spot (อิงคำว่า Spot หรือสัญลักษณ์ $)
		if strings.Contains(strings.ToLower(t), "spot") || strings.Contains(t, "$") {
			for j := i + 1; j < i+10 && j < len(tokens); j++ {
				num := re.FindString(tokens[j])
				val := parseToFloat(num)
				if val > 3000 && val < 6000 {
					cSpot = append(cSpot, fmt.Sprintf("%.2f", val))
					if len(cSpot) == 2 { break }
				}
			}
		}
	}

	if len(c96) >= 2 { result.Buy96, result.Sell96 = sortStr(c96[0], c96[1]) } else { result.Buy96, result.Sell96 = "N/A", "N/A" }
	if len(cSpot) >= 2 { result.SpotBid, result.SpotAsk = sortStr(cSpot[0], cSpot[1]) } else { result.SpotBid, result.SpotAsk = "N/A", "N/A" }

	return result, nil
}

func sortStr(a, b string) (string, string) {
	if parseToFloat(a) > parseToFloat(b) { return b, a }
	return a, b
}

func formatWithComma(n float64) string {
	s := strconv.FormatFloat(n, 'f', 0, 64)
	res := ""
	for i, j := len(s)-1, 0; i >= 0; i-- {
		res = string(s[i]) + res
		j++
		if j == 3 && i > 0 { res = "," + res; j = 0 }
	}
	return res
}

func parseToFloat(s string) float64 {
	clean := strings.ReplaceAll(s, ",", "")
	val, _ := strconv.ParseFloat(clean, 64)
	return val
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
