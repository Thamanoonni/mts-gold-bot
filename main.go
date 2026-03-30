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
			fmt.Fprintf(w, "Gold & Stock Bot V9.5 - Ready")
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
	ttw := fetchPrice("TTW.BK")
	scb := fetchPrice("SCB.BK")
	tisco := fetchPrice("TISCO.BK")
	neo := fetchPrice("NEO.BK")
	nyt := fetchPrice("NYT.BK")
	whair := fetchPrice("WHAIR.BK")
	
	// ดึงราคาทอง Spot (XAUUSD=X ตรงกับ Dime!)
	spot := fetchPrice("XAUUSD=X")

	report := fmt.Sprintf("🏆 **รายงานราคาประจำวัน (V9.5)**\n📅 %s\n\n"+
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

func fetchPrice(symbol string) string {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s", symbol)
	content := getRawHTML(url)
	re := regexp.MustCompile(`"regularMarketPrice":([0-9.]+)`)
	m := re.FindStringSubmatch(content)
	if len(m) > 1 { return m[1] }
	return "N/A"
}

func getRawHTML(url string) string {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil { return "" }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
