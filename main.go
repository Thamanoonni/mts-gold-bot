package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	TelegramBotToken = "8479186732:AAEtkVtmzwCu4yI5a-HvBBlaVjnI5djvAA8"
	TelegramChatID   = 8490072815
	// เจาะจงไปที่ API ราคาของ MTS โดยตรง (ไม่ต้องเปิด Browser)
	APIURL      = "https://www.mtsgold.co.th/mts-price-sm/p/price.php"
	HistoryFile = "gold_history_v6.json"
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

var bkkZone = time.FixedZone("BKK", 7*3600)

func main() {
	var err error
	bot, err = tgbotapi.NewBotAPI(TelegramBotToken)
	if err != nil {
		log.Panic(err)
	}

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("MTS Gold API-Bot V6 is running"))
		})
		port := os.Getenv("PORT")
		if port == "" { port = "8080" }
		http.ListenAndServe(":"+port, nil)
	}()

	go func() {
		processAndSend()
		ticker := time.NewTicker(30 * time.Minute) // ปรับมาเช็คทุก 30 นาที
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
	newData, err := fetchDataFromAPI()
	if err != nil {
		bot.Send(tgbotapi.NewMessage(TelegramChatID, "⚠️ API Error: "+err.Error()))
		return
	}
	
	timeNowTH := time.Now().In(bkkZone).Format("02/01/2006 15:04")
	text := fmt.Sprintf("🏆 **รายงานราคาทองคำ (Dime! & MTS)**\n📅 %s\n\n"+
		"🇹🇭 **ทองไทย 96.5%%**\n"+
		"🟢 รับซื้อ: %s\n🔴 ขายออก: %s\n\n"+
		"🌎 **Gold Spot (Dime!)**\n"+
		"🟢 Bid: %s\n🔴 Ask: %s",
		timeNowTH, newData.Buy96, newData.Sell96, newData.SpotBid, newData.SpotAsk,
	)

	// ระบบ Alert (เหมือนเดิม)
	currentSpotAsk := parseToFloat(newData.SpotAsk)
	todayStr := time.Now().In(bkkZone).Format("2006-01-02")
	for _, target := range TargetSpotPrices {
		if alertedSpotToday[target] != todayStr {
			if target < 4189 && currentSpotAsk <= target && currentSpotAsk > 0 {
				sendAlert(fmt.Sprintf("🚨 **ALERT: ทองโลกย่อถึงไม้ช้อน!**\n\nราคา: **%s** USD\nเป้า: %s USD", newData.SpotAsk, fmt.Sprintf("%.2f", target)))
				alertedSpotToday[target] = todayStr
			}
			if target > 4189 && currentSpotAsk >= target {
				sendAlert(fmt.Sprintf("🚀 **ALERT: ทองโลกพุ่งทะลุแนวต้าน!**\n\nราคา: **%s** USD\nเป้า: %s USD", newData.SpotAsk, fmt.Sprintf("%.2f", target)))
				alertedSpotToday[target] = todayStr
			}
		}
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

func fetchDataFromAPI() (MTSData, error) {
	var result MTSData
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(APIURL)
	if err != nil { return result, err }
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// เนื่องจาก API นี้อาจจะส่งมาเป็นก้อนข้อความ/HTML เราจะใช้การดึงค่าแบบง่ายๆ
	result.Buy96 = findValue(bodyStr, "96.5", 0)
	result.Sell96 = findValue(bodyStr, "96.5", 1)
	result.SpotBid = findValue(bodyStr, "Spot", 0)
	result.SpotAsk = findValue(bodyStr, "Spot", 1)

	return result, nil
}

func findValue(data, key string, order int) string {
	// Logic การหาตัวเลขที่อยู่หลังคำสำคัญ
	idx := strings.Index(data, key)
	if idx == -1 { return "N/A" }
	
	sub := data[idx:]
	parts := strings.Fields(sub)
	count := 0
	for _, p := range parts {
		clean := ""
		for _, r := range p {
			if (r >= '0' && r <= '9') || r == '.' || r == ',' { clean += string(r) }
		}
		if len(clean) > 3 {
			if count == order { return clean }
			count++
		}
	}
	return "N/A"
}

func parseToFloat(s string) float64 {
	clean := strings.ReplaceAll(s, ",", "")
	val, _ := strconv.ParseFloat(clean, 64)
	return val
}
