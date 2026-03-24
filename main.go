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
			fmt.Fprintf(w, "Gold & Stock Bot V9.0 - Active")
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
	
	// --- ดึงข้อมูลทอง ---
	thaiBuy, thaiSell := fetchThaiGold()
	spotPrice := fetchSpotGold()
	
	// คำนวณกำไรทอง (ทุน 4,189.92)
	profitGold := ""
	if s := strings.ReplaceAll(spotPrice, ",", ""); s != "N/A" {
		if cur, err := fmt.Sscanf(s, "%f", new(float64)); err == nil || cur > 0 {
			// ดึงค่ามาคำนวณแบบง่ายๆ
			re := regexp.MustCompile(`[0-9.]+`)
			valStr := re.FindString(s)
			var val float64
			fmt.Sscanf(valStr, "%f", &val)
			diff := val - 4189.92
			if diff > 0 { profitGold = fmt.Sprintf("\n📈 **กำไรทอง: +%.2f USD**", diff) } else { profitGold = fmt.Sprintf("\n📉 **ทองติดลบ: %.2f USD**", diff) }
		}
	}

	// --- ดึงข้อมูลหุ้น ---
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
		"💰 ราคา: **%s** USD%s\n\n"+
		"📈 **พอร์ตหุ้นปันผล**\n%s",
		timeNow, thaiBuy, thaiSell, spotPrice, profitGold, stockReport,
	)

	msg := tgbotapi.NewMessage(TelegramChatID, report)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func fetchThaiGold() (string, string) {
	content := getHTML("https://www.goldtraders.or.th/")
	re := regexp.MustCompile(`[0-9]{2},[0-9]{3}`)
	m := re.FindAllString(content, -1)
	if len(m) >= 2 { return m[0], m[1] }
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
	// ดึงราคาจาก SET ผ่านหน้าเว็บแบบง่าย
	url := fmt.Sprintf("https://www.set.or.th/th/market/product/stock/quote/%s/price", symbol)
	content := getHTML(url)
	// ค้นหาราคาล่าสุดใน HTML (Logic แบบกวาดหาตัวเลขหลังข้อความราคาล่าสุด)
	re := regexp.MustCompile(`"lastPrice":([0-9.]+)`)
	m := re.FindStringSubmatch(content)
	if len(m) > 1 { return m[1] }
	return "รอตลาด"
}

func getHTML(url string) string {
	c := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := c.Do(req)
	if err != nil { return "" }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
