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
			fmt.Fprintf(w, "Gold & Stock Bot V10.4 - All-Star Dividend")
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
			go sendReport(bot) // ทำงานแบบสายฟ้าแลบ ไม่รอคิว
		}
	}
}

func sendReport(bot *tgbotapi.BotAPI) {
	bkk, _ := time.LoadLocation("Asia/Bangkok")
	timeNow := time.Now().In(bkk).Format("02/01/2006 15:04")
	
	// ดึงราคาแบบ Anti-Delay
	spot := getPrice("PAXG-USD")
	scb := getPrice("SCB.BK")
	tisco := getPrice("TISCO.BK")
	
	ttw := getPrice("TTW.BK")
	whair := getPrice("WHAIR.BK")
	neo := getPrice("NEO.BK")
	nyt := getPrice("NYT.BK")
	rojna := getPrice("ROJNA.BK")
	ptt := getPrice("PTT.BK")
	advanc := getPrice("ADVANC.BK")
	ap := getPrice("AP.BK")

	// จัดรูปแบบข้อความแยกกลุ่ม Bank และ หุ้นปันผล
	report := fmt.Sprintf("🏆 **รายงานราคา (V10.4 All-Star Portfolio)**\n📅 %s\n\n"+
		"🌎 **Gold Spot (Dime!)**\n💰 ราคา: **%s** USD/oz\n\n"+
		"🏦 **กลุ่มธนาคาร (Bank)**\n"+
		"🔹 SCB    : **%s** บาท\n"+
		"🔹 TISCO  : **%s** บาท\n\n"+
		"📈 **พอร์ตหุ้นปันผล (เป้าหมาย)**\n"+
		"🔹 TTW    : **%s** บาท (เป้า: 9.00)\n"+
		"🔹 WHAIR  : **%s** บาท (เป้า: 6.45)\n"+
		"🔹 NEO    : **%s** บาท (เป้า: 18.50)\n"+
		"🔹 NYT    : **%s** บาท (เป้า: 4.10)\n"+
		"🔹 ROJNA  : **%s** บาท (เป้า: 5.00)\n"+
		"🔹 PTT    : **%s** บาท (เป้า: 32.50)\n"+
		"🔹 ADVANC : **%s** บาท (เป้า: 205.00)\n"+
		"🔹 AP     : **%s** บาท (เป้า: 10.50)",
		timeNow, spot, scb, tisco, ttw, whair, neo, nyt, rojna, ptt, advanc, ap,
	)

	msg := tgbotapi.NewMessage(TelegramChatID, report)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func getPrice(symbol string) string {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1m&_=%d", symbol, time.Now().Unix())
	client := &http.Client{Timeout: 5 * time.Second}
	
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/122.0.0.0 Safari/537.36")
		
		resp, err := client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			re := regexp.MustCompile(`"regularMarketPrice":([0-9.]+)`)
			m := re.FindStringSubmatch(string(body))
			if len(m) > 1 {
				return m[1]
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "N/A"
}
