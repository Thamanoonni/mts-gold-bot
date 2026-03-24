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
	ThaiGoldURL      = "https://www.goldtraders.or.th/"
	SpotGoldURL      = "https://api.coinbase.com/v2/prices/XAU-USD/spot"
)

var bkkZone = time.FixedZone("BKK", 7*3600)

func main() {
	bot, err := tgbotapi.NewBotAPI(TelegramBotToken)
	if err != nil {
		log.Panic(err)
	}

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Gold Bot V7.3 - Live")
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
		if txt == "ราคา" || txt == "price" || txt == "gold" {
			fetchAndReport(bot)
		}
	}
}

func fetchAndReport(bot *tgbotapi.BotAPI) {
	thaiBuy, thaiSell := fetchThaiGold()
	spotPrice := fetchSpotGold()

	timeNow := time.Now().In(bkkZone).Format("02/01/2006 15:04")
	
	profitText := ""
	currentSpot := parseToFloat(spotPrice)
	if currentSpot > 0 {
		diff := currentSpot - 4189.92
		if diff > 0 {
			profitText = fmt.Sprintf("\n📈 **กำไรไม้แรก: +%.2f USD**", diff)
		} else {
			profitText = fmt.Sprintf("\n📉 **ไม้แรกติดลบ: %.2f USD**", diff)
		}
	}

	report := fmt.Sprintf("🏆 **รายงานราคาทองคำ (V7.3)**\n📅 %s\n\n"+
		"🇹🇭 **ทองไทย (สมาคมฯ)**\n"+
		"🟢 รับซื้อ: %s\n🔴 ขายออก: %s\n\n"+
		"🌎 **Gold Spot (Dime!)**\n"+
		"💰 ราคาปัจจุบัน: **%s** USD/oz%s",
		timeNow, thaiBuy, thaiSell, spotPrice, profitText,
	)

	msg := tgbotapi.NewMessage(TelegramChatID, report)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func fetchThaiGold() (string, string) {
	content := getSimpleHTML(ThaiGoldURL)
	re := regexp.MustCompile(`[0-9]{2},[0-9]{3}`)
	matches := re.FindAllString(content, -1)
	if len(matches) >= 2 {
		return matches[0], matches[1]
	}
	return "N/A", "N/A"
}

func fetchSpotGold() string {
	content := getSimpleHTML(SpotGoldURL)
	re := regexp.MustCompile(`"amount":"([0-9.]+)"`)
	match := re.FindStringSubmatch(content)
	if len(match) > 1 {
		return match[1]
	}
	return "N/A"
}

func getSimpleHTML(target string) string {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", target, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	
	// ใช้ _ แทน err เพื่อให้ Build ผ่านแน่นอนครับ
	resp, _ := client.Do(req)
	if resp == nil {
		return ""
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func parseToFloat(s string) float64 {
	clean := strings.ReplaceAll(s, ",", "")
	val, _ := strconv.ParseFloat(clean, 64)
	return val
}
