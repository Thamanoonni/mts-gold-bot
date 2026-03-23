package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// --- Config ---
const (
	TelegramBotToken = "8479186732:AAEtkVtmzwCu4yI5a-HvBBlaVjnI5djvAA8"
	TelegramChatID   = 8490072815
	TargetURL        = "https://www.mtsgold.co.th/mts-price-sm/"
	HistoryFile      = "gold_history.json"
)

// 🎯 ตั้งราคาเป้าหมายหลายระดับ (เช็คกับ "ราคาขายออก")
var TargetPrices = []float64{67500.0, 65000.0}

var alertedToday = make(map[float64]string)
var bot *tgbotapi.BotAPI

type MTSData struct {
	Buy  string `json:"buy"`
	Sell string `json:"sell"`
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
		log.Panic("❌ เชื่อมต่อ Telegram ไม่ได้: ", err)
	}

	fmt.Println("🤖 Bot Online:", bot.Self.UserName)

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("MTS Gold Bot (96.5%) is Active!"))
		})
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
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
		if update.Message == nil {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(update.Message.Text))
		if text == "ราคา" || text == "price" || text == "gold" {
			processAndSend()
		}
	}
}

func processAndSend() {
	newData, err := scrapeMTSWithChrome()
	var text string

	if err != nil {
		text = "⚠️ เกิดข้อผิดพลาดในการดึงข้อมูล: `" + err.Error() + "`"
	} else {
		updateHistoryLogic(newData)

		// 🎯 แก้ไข: ดึงตัวเลข "ขายออก (Sell)" มาตรวจสอบแทนรับซื้อ
		currentSell := parseToFloat(newData.Sell)
		todayStr := time.Now().In(bkkZone).Format("2006-01-02")

		if currentSell > 0 {
			for _, target := range TargetPrices {
				// 🎯 เช็คว่าราคาขายออก ร่วงลงมาถึงเป้าหมายหรือยัง
				if currentSell <= target {
					if alertedToday[target] != todayStr {
						alertText := fmt.Sprintf("🚨 **ALERT: ราคาทองร่วงถึงไม้เป้าหมายแล้ว!** 🚨\n\n"+
							"ราคาขายออกปัจจุบัน: **%s** บาท\n"+
							"(เป้าหมายที่คุณตั้งไว้: %s บาท)\n\n"+
							"เตรียมพิจารณาเข้าซื้อได้เลยครับ!", newData.Sell, addCommaFloat(target))

						msgAlert := tgbotapi.NewMessage(TelegramChatID, alertText)
						msgAlert.ParseMode = "Markdown"
						bot.Send(msgAlert)

						alertedToday[target] = todayStr
					}
				}
			}
		}

		diffBuy := getDiffText(newData.Buy, history.YesterdayData.Buy)
		diffSell := getDiffText(newData.Sell, history.YesterdayData.Sell)

		if history.YesterdayData.Buy == "" {
			diffBuy, diffSell = "(🆕)", "(🆕)"
		}

		timeNowTH := time.Now().In(bkkZone).Format("02/01/2006 15:04")

		text = fmt.Sprintf("🏆 **ราคาทอง MTS Gold (96.5%%)**\n📅 %s\n(เทียบราคาปิดเมื่อวาน)\n\n"+
			"🟢 รับซื้อ: %s %s\n"+
			"🔴 ขายออก: %s %s",
			timeNowTH,
			newData.Buy, diffBuy,
			newData.Sell, diffSell,
		)
	}

	msg := tgbotapi.NewMessage(TelegramChatID, text)
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func scrapeMTSWithChrome() (MTSData, error) {
	var result MTSData
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"),
		chromedp.Flag("window-size", "1920,1080"),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	var bodyText string
	err := chromedp.Run(ctx,
		chromedp.Navigate(TargetURL),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Sleep(4*time.Second),
		chromedp.Evaluate(`document.body.innerText`, &bodyText),
	)
	if err != nil {
		return result, fmt.Errorf("เชื่อมต่อเว็บไม่ได้: %v", err)
	}

	tokens := strings.Fields(bodyText)
	var startIndex = -1
	
	for i, token := range tokens {
		if strings.Contains(token, "96.5") {
			startIndex = i
			break
		}
	}

	if startIndex == -1 {
		return result, fmt.Errorf("ไม่พบคำว่า '96.5' บนเว็บ")
	}

	var candidates []string
	for j := startIndex; j < len(tokens) && j < startIndex+30; j++ {
		cleanToken := strings.ReplaceAll(tokens[j], ",", "")
		cleanToken = strings.ReplaceAll(cleanToken, ".", "")
		
		if isNumeric(cleanToken) {
			val := parseToFloat(cleanToken)
			if val > 20000 && val < 100000 {
				candidates = append(candidates, tokens[j])
				if len(candidates) == 2 {
					break 
				}
			}
		}
	}
	
	if len(candidates) >= 2 {
		result.Buy = candidates[0]
		result.Sell = candidates[1]
		
		buyF := parseToFloat(result.Buy)
		sellF := parseToFloat(result.Sell)
		if buyF > sellF && sellF > 0 {
			result.Buy, result.Sell = result.Sell, result.Buy
		}
		return result, nil
	}

	return result, fmt.Errorf("หาตัวเลขเจอ %d ตัว", len(candidates))
}

func updateHistoryLogic(newData MTSData) {
	todayStr := time.Now().In(bkkZone).Format("2006-01-02")
	if history.CurrentDate == "" {
		history.CurrentDate = todayStr
		history.LastSeenData = newData
		history.YesterdayData = newData
		saveHistory(history)
		return
	}
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
	if err == nil {
		json.Unmarshal(file, &h)
	}
	return h
}

func saveHistory(h HistoryStore) {
	data, _ := json.MarshalIndent(h, "", " ")
	os.WriteFile(HistoryFile, data, 0644)
}

func getDiffText(currentStr, lastStr string) string {
	if lastStr == "" || currentStr == "-" {
		return ""
	}
	curr := parseToFloat(currentStr)
	last := parseToFloat(lastStr)
	diff := curr - last

	if diff > 0 {
		return fmt.Sprintf("(`🔺+%s`)", addCommaFloat(diff))
	}
	if diff < 0 {
		return fmt.Sprintf("(`🔻%s`)", addCommaFloat(diff))
	}
	return "(`➖คงที่`)"
}

func parseToFloat(s string) float64 {
	clean := strings.ReplaceAll(s, ",", "")
	val, _ := strconv.ParseFloat(clean, 64)
	return val
}

func addCommaFloat(n float64) string {
	in := int(n)
	s := strconv.Itoa(in)
	if in < 0 {
		s = s[1:]
	}
	nStr := ""
	count := 0
	for i := len(s) - 1; i >= 0; i-- {
		nStr = string(s[i]) + nStr
		count++
		if count == 3 && i > 0 {
			nStr = "," + nStr
			count = 0
		}
	}
	if int(n) < 0 {
		nStr = "-" + nStr
	}
	return nStr
}

func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
