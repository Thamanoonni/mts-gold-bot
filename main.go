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

var bkkZone = time.FixedZone("BKK", 7*3600)

func main() {
	bot, err := tgbotapi.NewBotAPI(TelegramBotToken)
	if err != nil {
		log.Panic(err)
	}

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Gold & Stock Bot V9.1 - Active")
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
			fetchAndReport(bot)
		}
	}
}

func fetchAndReport(bot *tgbotapi.BotAPI) {
	timeNow := time.Now().In(bkkZone).Format("02/01/2006 15:04")
	
	// 1. ดึงราคาทองโลก (Coinbase API - เสถียรที่สุด)
	spotPrice := fetchSpotGold()
	
	// 2. ดึงราคาทองไทย (ดึงผ่าน API สำรอง)
	thaiBuy, thaiSell := fetchThaiGold()
	
	// 3. ดึงราคาหุ้น (ใช้แหล่งข้อมูลที่ดึงง่ายขึ้น)
	stocks := []string{"TTW", "SCB", "TISCO", "WHAIR"}
	stockReport := ""
	for _, s := range stocks {
		price := fetchStockPrice(s)
		stockReport += fmt.Sprintf("🔹 %-6s: **%s** บาท\n", s, price)
	}

	report := fmt.Sprintf("🏆 **รายงานราคาประจำวัน**\n📅 %s\n\n"+
		"🇹🇭 **ทองไทย (สมาคมฯ)**\n"+
		"🟢 ซื้อ: %s | 🔴 ขาย: %s\n\n"+
		"🌎 **Gold Spot (Dime!)**\n"+
		"💰 ราคา: **%s** USD/oz\n\n"+
		"📈 **พอร์ตหุ้นปันผล**\n%s",
		timeNow, thaiBuy, thaiSell, spotPrice, stockReport,
	)

	msg := tgbotapi.NewMessage(TelegramChatID, report)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func fetchThaiGold() (string, string) {
	// ใช้ API ราคาทองคำไทยที่เป็นมิตรกับบอท
	content := getHTML("https://thai-gold-api.vercel.app/latest")
	reBuy := regexp.MustCompile(`"buy":([0-9]+)`)
	reSell := regexp.MustCompile(`"sell":([0-9]+)`)
	
	buy := reBuy.FindStringSubmatch(content)
	sell := reSell.FindStringSubmatch(content)
	
	if len(buy) > 1 && len(sell) > 1 {
		return formatNumber(buy[1]), formatNumber(sell[1])
	}
	return "N/A", "N/A"
}

func fetchSpotGold() string {
	content := getHTML("https://api.coinbase.com/v2/prices/XAU-USD/spot")
	re := regexp.MustCompile(`"amount":"([0-9.]+)"`)
	m := re.FindStringSubmatch(content)
	if len(m) > 1 { return m[1] }
	return "N/A"
}

func fetchStockPrice(symbol string) string {
	// ดึงราคาหุ้นผ่าน API ที่ไม่ต้องใช้ Browser
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s.BK", symbol)
	content := getHTML(url)
	re := regexp.MustCompile(`"regularMarketPrice":([0-9.]+)`)
	m := re.FindStringSubmatch(content)
	if len(m) > 1 { return m[1] }
	return "รอตลาด"
}

func getHTML(url string) string {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := client.Do(req)
	if err != nil { return "" }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func formatNumber(s string) string {
	if len(s) <= 3 { return s }
	res := ""
	for i, j := len(s)-1, 0; i >= 0; i-- {
		res = string(s[i]) + res
		j++
		if j == 3 && i > 0 { res = "," + res; j = 0 }
	}
	return res
}
