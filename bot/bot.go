package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"taxibot/config"
	"taxibot/database"
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
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		msg := update.Message
		if msg.IsCommand() {
			b.handleCommand(msg)
		} else if msg.Chat.IsPrivate() {
			b.handlePrivate(msg)
		} else if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
			b.handleGroup(msg)
		}
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

func (b *Bot) sendToTarget(from *tgbotapi.User, text string) error {
	targetID := b.db.GetTargetGroup()
	if targetID == 0 {
		return fmt.Errorf("target group not set")
	}
	caption := fmt.Sprintf("Yangi mijoz\n\n%s\n\n%s", text, b.profileName(from))

	// Try with profile button first
	msg := tgbotapi.NewMessage(targetID, caption)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Profilga o'tish", b.profileURL(from)),
		),
	)
	if _, err := b.api.Send(msg); err == nil {
		return nil
	}

	// Fallback: send plain text without button
	plain := tgbotapi.NewMessage(targetID, caption)
	if _, err := b.api.Send(plain); err != nil {
		log.Printf("Failed to send to target group: %v", err)
		return err
	}
	return nil
}

func (b *Bot) handlePrivate(msg *tgbotapi.Message) {
	if !b.db.IsBotEnabled() {
		return
	}
	if err := b.sendToTarget(msg.From, msg.Text); err != nil {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "Xatolik yuz berdi. Qaytadan urinib ko'ring."))
		return
	}
	b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "Buyurtmangiz taksichilarga yuborildi! Tez orada siz bilan bog'lanishadi."))
}

func (b *Bot) handleGroup(msg *tgbotapi.Message) {
	if !b.db.IsBotEnabled() || !b.db.IsMonitored(msg.Chat.ID) {
		return
	}
	// Skip anonymous admins (sent on behalf of a chat) and bots
	if msg.SenderChat != nil {
		return
	}
	if msg.From == nil || msg.From.IsBot {
		return
	}
	if b.isGroupAdmin(msg.Chat.ID, msg.From.ID) {
		return
	}

	if msg.Text != "" {
		b.sendToTarget(msg.From, msg.Text)
	}

	b.api.Request(tgbotapi.NewDeleteMessage(msg.Chat.ID, msg.MessageID))

	firstName := strings.ReplaceAll(msg.From.FirstName, " ", "_")
	noticeText := fmt.Sprintf(
		"🌟 ASSALOMU ALAYKUM XURMATLI #%s\nZAKASINGIZ QABUL QILINDI📨\nSHAFYORLAR SIZGA ALOQAGA CHIQISHADI ☎️\nBIZNI TANLAGANINGIZ UCHUN RAXMAT💥",
		firstName,
	)
	b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, noticeText))
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
	case "new":
		b.cmdNew(msg)
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

const newOrderUsage = "Foydalanish:\n" +
	"/new\n" +
	"ID: <mijozning Telegram ID raqami>\n" +
	"Ism: <ism familiya>\n" +
	"Username: <username, ixtiyoriy>\n" +
	"Matn: <buyurtma matni: manzil, telefon va h.k.>"

// cmdNew lets an admin (or an automated user-bot listed in ADMIN_IDS) submit
// order data collected elsewhere, using the same "Yangi mijoz" template and
// profile-button logic as organic group/private messages.
func (b *Bot) cmdNew(msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		return
	}
	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.reply(msg, newOrderUsage)
		return
	}
	customer, text, err := parseNewOrderArgs(args)
	if err != nil {
		b.reply(msg, fmt.Sprintf("Xatolik: %s\n\n%s", err.Error(), newOrderUsage))
		return
	}
	if err := b.sendToTarget(customer, text); err != nil {
		b.reply(msg, "Xatolik yuz berdi: maxsus guruhga yuborib bo'lmadi.")
		return
	}
	b.reply(msg, "✅ Buyurtma maxsus guruhga yuborildi.")
}

// parseNewOrderArgs parses the labeled-line format expected by /new:
//
//	ID: 123456789
//	Ism: Aziz Karimov
//	Username: azizkarimov
//	Matn: Chilonzordan Yunusobodga, tel: +998901234567
//
// "Matn:" marks the start of the free-text order body and may span
// multiple lines; it must be the last field.
func parseNewOrderArgs(args string) (*tgbotapi.User, string, error) {
	var id int64
	var ism, username string
	var matnLines []string
	inMatn := false

	for _, line := range strings.Split(args, "\n") {
		if inMatn {
			matnLines = append(matnLines, line)
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "ID:"):
			v := strings.TrimSpace(strings.TrimPrefix(trimmed, "ID:"))
			parsed, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, "", fmt.Errorf("ID noto'g'ri: %q", v)
			}
			id = parsed
		case strings.HasPrefix(trimmed, "Ism:"):
			ism = strings.TrimSpace(strings.TrimPrefix(trimmed, "Ism:"))
		case strings.HasPrefix(trimmed, "Username:"):
			username = strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(trimmed, "Username:")), "@")
		case strings.HasPrefix(trimmed, "Matn:"):
			inMatn = true
			if first := strings.TrimSpace(strings.TrimPrefix(trimmed, "Matn:")); first != "" {
				matnLines = append(matnLines, first)
			}
		}
	}

	if id == 0 {
		return nil, "", fmt.Errorf("ID ko'rsatilmagan")
	}
	if ism == "" {
		return nil, "", fmt.Errorf("Ism ko'rsatilmagan")
	}
	text := strings.TrimSpace(strings.Join(matnLines, "\n"))
	if text == "" {
		return nil, "", fmt.Errorf("Matn ko'rsatilmagan")
	}

	firstName, lastName := ism, ""
	if parts := strings.SplitN(ism, " ", 2); len(parts) == 2 {
		firstName, lastName = parts[0], parts[1]
	}

	customer := &tgbotapi.User{
		ID:        id,
		FirstName: firstName,
		LastName:  lastName,
		UserName:  username,
	}
	return customer, text, nil
}
