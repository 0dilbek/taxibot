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
		if update.Message == nil {
			continue
		}
		b.handleMessage(update.Message)
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

func (b *Bot) forwardToTarget(msg *tgbotapi.Message) {
	targetID := b.db.GetTargetGroup()
	if targetID == 0 {
		return
	}

	// Copy the original message first, then attach a profile button in a follow-up
	fwd := tgbotapi.NewForward(targetID, msg.Chat.ID, msg.MessageID)
	sent, err := b.api.Send(fwd)
	if err != nil {
		log.Printf("Failed to forward to target: %v", err)
		return
	}

	// Send profile button as a reply to the forwarded message
	reply := tgbotapi.NewMessage(targetID, b.profileName(msg.From))
	reply.ReplyToMessageID = sent.MessageID
	reply.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Profilga o'tish", b.profileURL(msg.From)),
		),
	)
	if _, err := b.api.Send(reply); err != nil {
		log.Printf("Failed to send profile button: %v", err)
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
	if msg.IsCommand() {
		b.handleCommand(msg)
		return
	}

	if !b.db.IsBotEnabled() {
		return
	}

	// Forward order to target group
	b.sendToTarget(msg.From, msg.Text)
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
	if b.isGroupAdmin(msg.Chat.ID, msg.From.ID) {
		return
	}

	// Forward to target group before deleting
	b.forwardToTarget(msg)

	// Delete message from group
	del := tgbotapi.NewDeleteMessage(msg.Chat.ID, msg.MessageID)
	if _, err := b.api.Request(del); err != nil {
		log.Printf("Failed to delete message: %v", err)
	}

	// Notify user via DM (may fail if user hasn't started the bot)
	dm := tgbotapi.NewMessage(msg.From.ID,
		"Guruhda buyurtma berish taqiqlangan. Iltimos, bot orqali buyurtma bering: qayerdan, qayerga va telefon raqamingizni yozing.")
	b.api.Send(dm)
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
