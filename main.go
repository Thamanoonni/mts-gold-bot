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
			fmt.Fprintf(w, "Gold & Stock Bot V9.7 - Running")
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
	ttw := fetchPrice("https://query1.finance.yahoo.com/v8/finance/chart/TTW.BK")
	scb := fetchPrice("https://query1.finance.yahoo.com/v8/finance/chart/SCB.BK")
	tisco := fetchPrice("https://query1.finance.yahoo.com/v8/finance/chart/TISCO.BK")
	neo := fetchPrice("https://query1.finance.yahoo.com/v8/finance/chart/NEO.BK")
	nyt := fetchPrice("https://query1.finance.yahoo.com/v8/finance/chart/NYT.BK")
	whair := fetchPrice("https://query1.finance.yahoo.com/v8/finance/chart/WHAIR.BK")
	
	// ดึงราคาทอง Spot จาก Binance API (PAXG/USDT = 1 oz Gold)
	spot := fetchGoldBinance()

	report := fmt.Sprintf("🏆 **รายงานราคาประจำวัน (V9.7)**\n📅 %s\n\n"+
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

func fetchPrice(url string) string {
	content := getRaw(url)
	re := regexp.MustCompile(`"regularMarketPrice":([0-9.]+)`)
	m := re.FindStringSubmatch(content)
	if len(m) > 1 { return m[1] }
	return "N/A"
}

func fetchGoldBinance() string {
	// ใช้ Binance API ดึงราคา PAXG (ซึ่งผูกกับราคาทองคำจริง 1:1)
	url := "https://api.binance.com/api/v3/ticker/price?symbol=PAXGUSDT"
	content := getRaw(url)
	re := regexp.MustCompile(`"price":"([0-9.]+)"`)
	m := re.FindStringSubmatch(content)
	if len(m) > 1 { 
		// ตัดทศนิยมให้เหลือ 2 ตำแหน่งเพื่อความสวยงาม
		var val float64
		fmt.Sscanf(m[1], "%f", &val)
		return fmt.Sprintf("%.2f", val)
	}
	return "N/A"
}

func getRaw(url string) string {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil { return "" }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
