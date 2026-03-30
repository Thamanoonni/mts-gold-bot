package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	TelegramBotToken = "8479186732:AAEtkVtmzwCu4yI5a-HvBBlaVjnI5djvAA8"
	TelegramChatID   = 8490072815
)

func main() {
	bot, err := tgbotapi.NewBotAPI(TelegramBotToken)
	if err != nil {
		log.Panic(err)
	}

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Gold & Stock Bot V10.0 - Ready")
		})
		port := os.Getenv("PORT")
		if port == "" { port = "8080" }
		http.ListenAndServe(":"+port, nil)
	}()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil { continue }
		txt := strings.ToLower(update.Message.Text)
		if txt == "ราคา" || txt == "price" || txt == "gold" || txt == "stock" {
			sendReport(bot)
		}
	}
}

func sendReport(bot *tgbotapi.BotAPI) {
	bkk, _ := time.LoadLocation("Asia/Bangkok")
	timeNow := time.Now().In(bkk).Format("02/01/2006 15:04")
	
	// ดึงราคาหุ้น
	ttw := getPrice("TTW.BK")
	scb := getPrice("SCB.BK")
	tisco := getPrice("TISCO.BK")
	neo := getPrice("NEO.BK")
	nyt := getPrice("NYT.BK")
	whair := getPrice("WHAIR.BK")
	
	// ดึงราคาทอง Spot (ดึงจาก CryptoCompare API - เสถียรและเป็น Spot จริง)
	spot := getGoldSpot()

	report := fmt.Sprintf("🏆 **รายงานราคาประจำวัน (V10.0)**\n📅 %s\n\n"+
		"🌎 **Gold Spot (Dime!)**\n💰 ราคา: **%s** USD/oz\n\n"+
		"📈 **พอร์ตหุ้นปันผล**\n"+
		"🔹 TTW   : **%s** บาท\n"+
		"🔹 SCB   : **%s** บาท\n"+
		"🔹 TISCO : **%s** บาท\n"+
		"🔹 NEO   : **%s** บาท\n"+
		"🔹 NYT   : **%s** บาท\n"+
		"🔹 WHAIR : **%s** บาท",
		timeNow, spot, ttw, scb, tisco, neo, nyt, whair,
	)

	msg := tgbotapi.NewMessage(TelegramChatID, report)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func getGoldSpot() string {
	// ดึงราคา XAU/USD ผ่าน CryptoCompare API ซึ่งเสถียรมากสำหรับ Bot
	url := "https://min-api.cryptocompare.com/data/price?fsym=XAU&tsyms=USD"
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil { return "N/A" }
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	re := regexp.MustCompile(`"USD":([0-9.]+)`)
	m := re.FindStringSubmatch(string(body))
	if len(m) > 1 {
		return m[1]
	}
	return "N/A"
}

func getPrice(symbol string) string {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s", symbol)
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	
	resp, err := client.Do(req)
	if err != nil { return "N/A" }
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	re := regexp.MustCompile(`"regularMarketPrice":([0-9.]+)`)
	m := re.FindStringSubmatch(string(body))
	if len(m) > 1 { return m[1] }
	return "N/A"
}
