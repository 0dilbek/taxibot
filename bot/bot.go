package bot

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"taxibot/config"
	"taxibot/database"
)

const (
	adImageURL = "https://cdn.upl.uz/posts/2025-06/7881302819_photo_2025-06-17_21-49-27.jpg"
	adText     = "✅ RASMIY BOTIMIZ ORQALI ISHONCHI TEZKOR TAXI BUYURTMA BERISHINGIZ MUMKIN BUNING UCHUN PASTDAGI TAXI BUYURTMA BERISH TUGMASINI BOSIB OSON TAXI ZAKAZ QILING 🚕"
)

type Bot struct {
	api *tgbotapi.BotAPI
	cfg *config.Config
	db  *database.DB
}

func New(cfg *config.Config, db *database.DB) *Bot {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Fatal("Failed to create bot:", err)
	}
	log.Printf("Authorized as @%s", api.Self.UserName)
	return &Bot{api: api, cfg: cfg, db: db}
}

func (b *Bot) Start() {
	// On restart — send ad immediately
	b.sendAd()

	// Daily scheduler
	go b.runScheduler()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		if update.Message.IsCommand() {
			b.handleCommand(update.Message)
		} else if update.Message.Chat.IsPrivate() {
			b.handlePrivate(update.Message)
		}
	}
}

// sendAd sends the advertisement photo to all monitored groups.
func (b *Bot) sendAd() {
	groups, err := b.db.ListGroups()
	if err != nil || len(groups) == 0 {
		log.Println("No monitored groups to send ad")
		return
	}

	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Buyurtma berish", "https://t.me/"+b.api.Self.UserName),
		),
	)

	for _, g := range groups {
		photo := tgbotapi.NewPhoto(g.ChatID, tgbotapi.FileURL(adImageURL))
		photo.Caption = adText
		photo.ReplyMarkup = markup
		if _, err := b.api.Send(photo); err != nil {
			log.Printf("Failed to send ad to group %d (%s): %v", g.ChatID, g.Title, err)
		}
	}
}

// runScheduler sends the ad once a day at a random time between 07:00 and 20:00.
func (b *Bot) runScheduler() {
	for {
		next := nextRandomTime()
		log.Printf("Next ad scheduled at: %s", next.Format("2006-01-02 15:04:05"))
		time.Sleep(time.Until(next))
		b.sendAd()
	}
}

// nextRandomTime returns a random time between 07:00 and 20:00 the next day.
func nextRandomTime() time.Time {
	now := time.Now()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	minSec := 7 * 3600
	maxSec := 20 * 3600
	randomSec := minSec + rand.Intn(maxSec-minSec)
	return tomorrow.Add(time.Duration(randomSec) * time.Second)
}

func (b *Bot) isAdmin(userID int64) bool {
	for _, id := range b.cfg.AdminIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func (b *Bot) profileURL(user *tgbotapi.User) string {
	if user.UserName != "" {
		return "https://t.me/" + user.UserName
	}
	return fmt.Sprintf("tg://user?id=%d", user.ID)
}

func (b *Bot) profileName(user *tgbotapi.User) string {
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if user.UserName != "" {
		return fmt.Sprintf("%s (@%s)", name, user.UserName)
	}
	return name
}

func (b *Bot) sendToTarget(from *tgbotapi.User, text string) {
	targetID := b.db.GetTargetGroup()
	if targetID == 0 {
		return
	}
	caption := fmt.Sprintf("Yangi mijoz\n\n%s\n\n%s", text, b.profileName(from))
	msg := tgbotapi.NewMessage(targetID, caption)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Profilga o'tish", b.profileURL(from)),
		),
	)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Failed to send to target group: %v", err)
	}
}

func (b *Bot) handlePrivate(msg *tgbotapi.Message) {
	if !b.db.IsBotEnabled() {
		return
	}
	b.sendToTarget(msg.From, msg.Text)
	reply := tgbotapi.NewMessage(msg.Chat.ID, "Buyurtmangiz taksichilarga yuborildi! Tez orada siz bilan bog'lanishadi.")
	b.api.Send(reply)
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		b.cmdStart(msg)
	case "add":
		b.cmdAdd(msg)
	case "rm":
		b.cmdRm(msg)
	case "change":
		b.cmdChange(msg)
	case "on":
		b.cmdOn(msg)
	case "off":
		b.cmdOff(msg)
	case "sendnow":
		b.cmdSendNow(msg)
	}
}

func (b *Bot) reply(msg *tgbotapi.Message, text string) {
	b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, text))
}

func (b *Bot) cmdStart(msg *tgbotapi.Message) {
	b.reply(msg, "Assalomu alaykum! Taxi buyurtma berish uchun quyidagi ma'lumotlarni bitta xabarda yuboring:\n\n"+
		"📍 Qayerdan\n🏁 Qayerga\n📞 Telefon raqam")
}

func (b *Bot) cmdAdd(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		return
	}
	if msg.Chat.IsPrivate() {
		b.reply(msg, "Bu buyruqni guruhda yuboring.")
		return
	}
	if err := b.db.AddGroup(msg.Chat.ID, msg.Chat.Title); err != nil {
		b.reply(msg, "Xatolik yuz berdi.")
		return
	}
	b.reply(msg, fmt.Sprintf("✅ Guruh ro'yxatga qo'shildi: %s", msg.Chat.Title))
}

func (b *Bot) cmdRm(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		return
	}
	if msg.Chat.IsPrivate() {
		b.reply(msg, "Bu buyruqni guruhda yuboring.")
		return
	}
	if err := b.db.RemoveGroup(msg.Chat.ID); err != nil {
		b.reply(msg, "Xatolik yuz berdi.")
		return
	}
	b.reply(msg, fmt.Sprintf("✅ Guruh ro'yxatdan chiqarildi: %s", msg.Chat.Title))
}

func (b *Bot) cmdChange(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		return
	}
	if msg.Chat.IsPrivate() {
		b.reply(msg, "Bu buyruqni maxsus guruhda yuboring.")
		return
	}
	if err := b.db.SetTargetGroup(msg.Chat.ID); err != nil {
		b.reply(msg, "Xatolik yuz berdi.")
		return
	}
	b.reply(msg, fmt.Sprintf("✅ Maxsus guruh o'zgartirildi: %s", msg.Chat.Title))
}

func (b *Bot) cmdOn(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		return
	}
	b.db.SetBotEnabled(true)
	b.reply(msg, "✅ Bot yoqildi.")
}

func (b *Bot) cmdOff(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		return
	}
	b.db.SetBotEnabled(false)
	b.reply(msg, "⛔ Bot o'chirildi.")
}

// cmdSendNow forces sending the ad immediately (admin only).
func (b *Bot) cmdSendNow(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		return
	}
	b.sendAd()
	b.reply(msg, "✅ Reklama xabari yuborildi.")
}
