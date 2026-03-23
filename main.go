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
	APIURL           = "https://www.mtsgold.co.th/mts-price-sm/p/price.php"
)

var bkkZone = time.FixedZone("BKK", 7*3600)

func main() {
	bot, err := tgbotapi.NewBotAPI(TelegramBotToken)
	if err != nil {
		log.Panic(err)
	}

	// Health check for Deployment
	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "MTS Gold API-Bot is running")
		})
		port := os.Getenv("PORT")
		if port == "" { port = "8080" }
		http.ListenAndServe(":"+port, nil)
	}()

	// Automatic Alert every 30 minutes
	go func() {
		for {
			processAndSend(bot)
			time.Sleep(30 * time.Minute)
		}
	}()

	// Command listener
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		if update.Message == nil { continue }
		txt := strings.ToLower(update.Message.Text)
		if txt == "ราคา" || txt == "price" || txt == "gold" {
			processAndSend(bot)
		}
	}
}

func processAndSend(bot *tgbotapi.BotAPI) {
	data, err := fetchData()
	if err != nil {
		msg := tgbotapi.NewMessage(TelegramChatID, "⚠️ ขัดข้อง: "+err.Error())
		bot.Send(msg)
		return
	}

	timeNow := time.Now().In(bkkZone).Format("02/01/2006 15:04")
	report := fmt.Sprintf("🏆 **รายงานราคาทองคำ (Dime! & MTS)**\n📅 %s\n\n"+
		"🇹🇭 **ทองไทย 96.5%%**\n"+
		"🟢 รับซื้อ: %s\n🔴 ขายออก: %s\n\n"+
		"🌎 **Gold Spot (USD/oz)**\n"+
		"🟢 Bid: %s\n🔴 Ask: %s",
		timeNow, data["buy96"], data["sell96"], data["bidSpot"], data["askSpot"],
	)

	msg := tgbotapi.NewMessage(TelegramChatID, report)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func fetchData() (map[string]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(APIURL)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	re := regexp.MustCompile(`[0-9,.]+`)
	tokens := strings.Fields(content)

	res := map[string]string{
		"buy96": "N/A", "sell96": "N/A",
		"bidSpot": "N/A", "askSpot": "N/A",
	}

	var found96 []string
	var foundSpot []string

	for i, t := range tokens {
		if strings.Contains(t, "96.5") {
			for j := i + 1; j < i+10 && j < len(tokens); j++ {
				num := re.FindString(tokens[j])
				if len(num) > 4 && !strings.Contains(num, ".") { // เลขทองไทย
					found96 = append(found96, num)
					if len(found96) == 2 { break }
				}
			}
		}
		if strings.Contains(strings.ToLower(t), "spot") {
			for j := i + 1; j < i+10 && j < len(tokens); j++ {
				num := re.FindString(tokens[j])
				if strings.Contains(num, ".") && len(num) > 4 { // เลข Spot (มีทศนิยม)
					foundSpot = append(foundSpot, num)
					if len(foundSpot) == 2 { break }
				}
			}
		}
	}

	if len(found96) >= 2 {
		res["buy96"], res["sell96"] = found96[0], found96[1]
	}
	if len(foundSpot) >= 2 {
		res["bidSpot"], res["askSpot"] = foundSpot[0], foundSpot[1]
	}

	return res, nil
}
