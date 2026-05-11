package bot

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"taxibot/config"
	"taxibot/database"
)

type Bot struct {
	api    *tgbotapi.BotAPI
	cfg    *config.Config
	db     *database.DB
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
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		msg := update.Message
		if msg == nil {
			msg = update.EditedMessage
		}
		if msg == nil {
			continue
		}
		b.handleMessage(msg)
	}
}

func (b *Bot) isAdmin(userID int64) bool {
	for _, id := range b.cfg.AdminIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func (b *Bot) isGroupAdmin(chatID, userID int64) bool {
	member, err := b.api.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	})
	if err != nil {
		return false
	}
	return member.IsAdministrator() || member.IsCreator()
}

func (b *Bot) profileURL(user *tgbotapi.User) string {
	if user == nil {
		return "https://t.me"
	}
	if user.UserName != "" {
		return "https://t.me/" + user.UserName
	}
	return fmt.Sprintf("tg://user?id=%d", user.ID)
}

func (b *Bot) profileName(user *tgbotapi.User) string {
	if user == nil {
		return "Noma'lum"
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name == "" {
		name = "Mijoz"
	}
	if user.UserName != "" {
		return fmt.Sprintf("%s (@%s)", name, user.UserName)
	}
	return name
}

func (b *Bot) hashtagName(user *tgbotapi.User) string {
	if user == nil {
		return "mijoz"
	}
	name := user.FirstName
	if name == "" {
		name = "mijoz"
	}
	// Remove spaces for hashtag compatibility
	return strings.ReplaceAll(name, " ", "_")
}

func (b *Bot) sendToTarget(msg *tgbotapi.Message) {
	targetID := b.db.GetTargetGroup()
	if targetID == 0 {
		log.Println("Warning: Target group not set. Use /change command in the target group.")
		return
	}

	from := msg.From
	name := b.profileName(from)
	url := b.profileURL(from)

	// Case 1: Simple text message - send as a single formatted message
	if msg.Text != "" {
		caption := fmt.Sprintf("🚖 Yangi mijoz: %s\n\n%s", name, msg.Text)
		m := tgbotapi.NewMessage(targetID, caption)
		m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("Profilga o'tish", url),
			),
		)
		if _, err := b.api.Send(m); err == nil {
			return
		}
	}

	// Case 2: Media with caption - try to copy it
	// Case 3: Other types (Location, Contact, etc.)
	// We send a header first to identify the customer, then copy the content
	header := fmt.Sprintf("🚖 Yangi mijoz: %s", name)
	infoMsg := tgbotapi.NewMessage(targetID, header)
	infoMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Profilga o'tish", url),
		),
	)
	b.api.Send(infoMsg)

	// Now copy the original message content (photo, location, etc.)
	copyMsg := tgbotapi.NewCopyMessage(targetID, msg.Chat.ID, msg.MessageID)
	if _, err := b.api.Request(copyMsg); err != nil {
		log.Printf("Failed to copy message: %v. Trying forward...", err)
		// Last resort: Forward the message
		fwd := tgbotapi.NewForward(targetID, msg.Chat.ID, msg.MessageID)
		b.api.Request(fwd)
	}
}


func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	// Private chat
	if msg.Chat.IsPrivate() {
		b.handlePrivate(msg)
		return
	}

	// Group / supergroup
	if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
		b.handleGroup(msg)
	}
}

func (b *Bot) handlePrivate(msg *tgbotapi.Message) {
	if msg.From == nil {
		return
	}
	if msg.IsCommand() {
		b.handleCommand(msg)
		return
	}

	if !b.db.IsBotEnabled() {
		return
	}

	// Forward order to target group
	b.sendToTarget(msg)
	
	reply := tgbotapi.NewMessage(msg.Chat.ID, "Buyurtmangiz taksichilarga yuborildi! Tez orada siz bilan bog'lanishadi.")
	b.api.Send(reply)
}

func (b *Bot) handleGroup(msg *tgbotapi.Message) {
	if msg.IsCommand() {
		b.handleCommand(msg)
		return
	}

	if !b.db.IsBotEnabled() {
		return
	}

	if !b.db.IsMonitored(msg.Chat.ID) {
		return
	}

	// Don't touch group admins' messages
	if msg.From != nil && b.isGroupAdmin(msg.Chat.ID, msg.From.ID) {
		return
	}

	// Send to target group NO MATTER WHAT (Text, Photo, Location, Contact, etc.)
	b.sendToTarget(msg)

	// Delete message from group
	del := tgbotapi.NewDeleteMessage(msg.Chat.ID, msg.MessageID)
	if _, err := b.api.Request(del); err != nil {
		log.Printf("Failed to delete message: %v", err)
	}

	// Notify in the group with bot link button and hashtag
	name := b.hashtagName(msg.From)
	noticeText := fmt.Sprintf("🚕 Assalomu alaykum #%s, xabaringiz yetkazildi. Taksi chaqirish uchun botimizga tashrif buyuring.\n\nBuyurtma berish tugmasini bosing hurmatli mijoz\n👇👇👇👇👇👇👇👇👇👇👇", name)
	
	notice := tgbotapi.NewMessage(msg.Chat.ID, noticeText)
	notice.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Buyurtma berish", "https://t.me/"+b.api.Self.UserName),
		),
	)
	b.api.Send(notice)
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
	}
}

func (b *Bot) reply(msg *tgbotapi.Message, text string) {
	m := tgbotapi.NewMessage(msg.Chat.ID, text)
	b.api.Send(m)
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
	title := msg.Chat.Title
	if err := b.db.AddGroup(msg.Chat.ID, title); err != nil {
		b.reply(msg, "Xatolik yuz berdi.")
		log.Printf("AddGroup error: %v", err)
		return
	}
	b.reply(msg, fmt.Sprintf("✅ Guruh ro'yxatga qo'shildi: %s", title))
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
