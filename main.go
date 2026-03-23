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
	HistoryFile      = "gold_history_v2.json" // เปลี่ยนชื่อไฟล์เล็กน้อยเพื่อรับระบบ 2 พอร์ต
)

// 🎯 ตั้งราคาเป้าหมาย ทองไทย 96.5% (เล็งราคาขายออก)
var TargetPrices96 = []float64{67500.0, 65000.0}

// 🎯 ตั้งราคาเป้าหมาย ทองโลก Spot Gold USD/oz (เล็งราคา Ask ที่เราจะซื้อ)
var TargetSpotPrices = []float64{4100.0, 4050.0, 4000.0}

var alerted96Today = make(map[float64]string)
var alertedSpotToday = make(map[float64]string)
var bot *tgbotapi.BotAPI

// โครงสร้างข้อมูล 2 ระบบ
type MTSData struct {
	Buy96   string `json:"buy_96"`
	Sell96  string `json:"sell_96"`
	SpotBid string `json:"spot_bid"`
	SpotAsk string `json:"spot_ask"`
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
			w.Write([]byte("MTS Gold Dual-Bot is Active!"))
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
		text = "⚠️ เกิดข้อผิดพลาดในการดึงข้อมูล:\n`" + err.Error() + "`"
	} else {
		updateHistoryLogic(newData)
		todayStr := time.Now().In(bkkZone).Format("2006-01-02")

		// 🚨 1. เช็คเป้าหมาย: ทองไทย 96.5%
		currentSell96 := parseToFloat(newData.Sell96)
		if currentSell96 > 0 {
			for _, target := range TargetPrices96 {
				if currentSell96 <= target {
					if alerted96Today[target] != todayStr {
						alertText := fmt.Sprintf("🚨 **ALERT (พอร์ตเกษียณ): ทองไทยถึงเป้าแล้ว!** 🚨\n\n"+
							"ราคาขายออก: **%s** บาท\n"+
							"(เป้าหมาย: %s บาท)", newData.Sell96, addCommaFloat(target))
						msgAlert := tgbotapi.NewMessage(TelegramChatID, alertText)
						msgAlert.ParseMode = "Markdown"
						bot.Send(msgAlert)
						alerted96Today[target] = todayStr
					}
				}
			}
		}

		// 🚨 2. เช็คเป้าหมาย: ทองโลก Spot Gold (Dime!)
		currentSpotAsk := parseToFloat(newData.SpotAsk)
		if currentSpotAsk > 0 {
			for _, target := range TargetSpotPrices {
				if currentSpotAsk <= target {
					if alertedSpotToday[target] != todayStr {
						alertText := fmt.Sprintf("🚨 **ALERT (พอร์ต Dime!): ทองโลกย่อถึงไม้ดักช้อนแล้ว!** 🚨\n\n"+
							"ราคา Spot (Ask): **%s** USD/oz\n"+
							"(เป้าหมาย: %s USD/oz)\n\n"+
							"เตรียมกระสุน 100,000 เข้าช้อนไม้ 2 ได้เลยครับคุณพ่อ!", newData.SpotAsk, addCommaFloat(target))
						msgAlert := tgbotapi.NewMessage(TelegramChatID, alertText)
						msgAlert.ParseMode = "Markdown"
						bot.Send(msgAlert)
						alertedSpotToday[target] = todayStr
					}
				}
			}
		}

		// 📝 เตรียมข้อความส่วนต่างราคา (เทียบเมื่อวาน)
		diffBuy96 := getDiffText(newData.Buy96, history.YesterdayData.Buy96)
		diffSell96 := getDiffText(newData.Sell96, history.YesterdayData.Sell96)
		diffSpotBid := getDiffText(newData.SpotBid, history.YesterdayData.SpotBid)
		diffSpotAsk := getDiffText(newData.SpotAsk, history.YesterdayData.SpotAsk)

		if history.YesterdayData.Buy96 == "" {
			diffBuy96, diffSell96, diffSpotBid, diffSpotAsk = "(🆕)", "(🆕)", "(🆕)", "(🆕)"
		}

		timeNowTH := time.Now().In(bkkZone).Format("02/01/2006 15:04")

		// 🏆 สรุปรายงาน 2 ระบบ
		text = fmt.Sprintf("🏆 **รายงานราคาทองคำ (2 ระบบ)**\n📅 %s\n\n"+
			"🇹🇭 **ทองคำแท่ง 96.5%%**\n"+
			"🟢 รับซื้อ: %s %s\n"+
			"🔴 ขายออก: %s %s\n\n"+
			"🌎 **Gold Spot (Dime!)**\n"+
			"🟢 Bid: %s %s\n"+
			"🔴 Ask: %s %s",
			timeNowTH,
			newData.Buy96, diffBuy96,
			newData.Sell96, diffSell96,
			newData.SpotBid, diffSpotBid,
			newData.SpotAsk, diffSpotAsk,
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
	
	// --- 1. ค้นหาทองไทย 96.5% ---
	var idx96 = -1
	for i, token := range tokens {
		if strings.Contains(token, "96.5") {
			idx96 = i
			break
		}
	}

	if idx96 != -1 {
		var cands96 []string
		for j := idx96; j < len(tokens) && j < idx96+30; j++ {
			cleanToken := strings.ReplaceAll(tokens[j], ",", "")
			if isNumeric(cleanToken) {
				val := parseToFloat(cleanToken)
				if val > 20000 && val < 100000 {
					cands96 = append(cands96, tokens[j])
					if len(cands96) == 2 {
						break 
					}
				}
			}
		}
		if len(cands96) >= 2 {
			result.Buy96 = cands96[0]
			result.Sell96 = cands96[1]
			// จัดเรียงให้น้อยอยู่หน้า (รับซื้อต้องถูกกว่าขายออก)
			if parseToFloat(result.Buy96) > parseToFloat(result.Sell96) {
				result.Buy96, result.Sell96 = result.Sell96, result.Buy96
			}
		}
	}

	// --- 2. ค้นหา Gold Spot (USD) ---
	// หลักการ: ดึงตัวเลขที่มีค่าระหว่าง 3,000 - 6,000 (เพื่อหลีกเลี่ยงปี ค.ศ. หรือราคาทองไทย)
	var candsSpot []string
	for _, token := range tokens {
		cleanToken := strings.ReplaceAll(token, ",", "")
		if isNumeric(cleanToken) {
			val := parseToFloat(cleanToken)
			if val > 3000.0 && val < 6000.0 { // ช่วงราคา Spot ปัจจุบัน
				candsSpot = append(candsSpot, token)
				if len(candsSpot) == 2 {
					break
				}
			}
		}
	}

	if len(candsSpot) >= 2 {
		result.SpotBid = candsSpot[0]
		result.SpotAsk = candsSpot[1]
		if parseToFloat(result.SpotBid) > parseToFloat(result.SpotAsk) {
			result.SpotBid, result.SpotAsk = result.SpotAsk, result.SpotBid
		}
	}

	if result.Buy96 == "" && result.SpotBid == "" {
		return result, fmt.Errorf("หาตัวเลขไม่เจอทั้ง 2 ระบบเลยครับ")
	}

	// ถ้าเจออย่างใดอย่างหนึ่งให้ถือว่าผ่าน (แสดงเป็น N/A ได้ถ้าระบบใดรวน)
	if result.Buy96 == "" { result.Buy96 = "N/A"; result.Sell96 = "N/A" }
	if result.SpotBid == "" { result.SpotBid = "N/A"; result.SpotAsk = "N/A" }

	return result, nil
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
	if lastStr == "" || currentStr == "-" || currentStr == "N/A" || lastStr == "N/A" {
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
	// รองรับจุดทศนิยมสำหรับ Spot Gold
	parts := strings.Split(fmt.Sprintf("%.2f", n), ".")
	intPart := parts[0]
	decPart := parts[1]

	sign := ""
	if strings.HasPrefix(intPart, "-") {
		sign = "-"
		intPart = intPart[1:]
	}

	nStr := ""
	count := 0
	for i := len(intPart) - 1; i >= 0; i-- {
		nStr = string(intPart[i]) + nStr
		count++
		if count == 3 && i > 0 {
			nStr = "," + nStr
			count = 0
		}
	}
	
	// ถ้าเป็นทศนิยม .00 ให้ตัดทิ้งเพื่อความสวยงาม (สำหรับทองไทย)
	if decPart == "00" {
		return sign + nStr
	}
	return sign + nStr + "." + decPart
}

func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
