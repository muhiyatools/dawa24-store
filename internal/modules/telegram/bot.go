package telegram

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// busyFor bounds the one-question-at-a-time lock. Longer than the assistant's
// own turn deadline, so a finished turn always releases it first; short enough
// that a process that died mid-turn does not silence the chat for long.
const busyFor = 3 * time.Minute

// HandleUpdate answers one Telegram update.
//
// It returns an error only for infrastructure failures. Everything a user can
// cause — an unknown command, an expired code, a missing permission — is an
// ordinary reply, so n8n never has to interpret a failure.
//
// Groups and channels are ignored entirely. An assistant answer is a pharmacy's
// or supplier's trading position; in a group it would be read by everyone in
// the chat, whoever they are.
func (s *Service) HandleUpdate(ctx context.Context, u Update) (Reply, error) {
	sys := database.AsSystem(ctx)

	if m := u.MyChatMember; m != nil {
		if m.Chat.Type == "private" {
			if first, err := s.repo.MarkUpdateProcessed(sys, u.UpdateID); err != nil || !first {
				return Reply{}, err
			}
			return Reply{}, s.onMembershipChange(sys, m)
		}
		return Reply{}, nil
	}

	if q := u.CallbackQuery; q != nil {
		return s.handleCallback(ctx, u.UpdateID, q)
	}

	msg := u.Message
	if msg == nil || msg.From == nil || msg.From.IsBot || msg.Chat.Type != "private" || msg.Chat.ID != msg.From.ID {
		return Reply{}, nil
	}
	first, err := s.repo.MarkUpdateProcessed(sys, u.UpdateID)
	if err != nil || !first {
		return Reply{}, err
	}

	chatID := msg.Chat.ID
	cmd, arg := parseCommand(msg.Text, s.cfg.BotUsername)
	if cmd == "start" && arg != "" {
		return s.handleLinkCode(sys, msg, arg)
	}

	link, err := s.repo.LiveLinkByTelegramUser(sys, msg.From.ID)
	if err != nil {
		return Reply{}, err
	}
	if link == nil {
		return replyTo(chatID, s.notLinkedText()), nil
	}
	if link.Status == LinkPending {
		if link.ConfirmExpiresAt != nil && s.now().After(*link.ConfirmExpiresAt) {
			return replyTo(chatID, "⌛ انتهت مهلة تأكيد الربط. أنشئ رابطاً جديداً من صفحة الإعدادات في Dawa24."), nil
		}
		return replyTo(chatID, "⏳ طلب الربط بانتظار تأكيدك. افتح صفحة الإعدادات في Dawa24 واضغط «تأكيد الربط»."), nil
	}
	if link.Status == LinkBlocked {
		// Writing to the bot is the clearest possible sign it is unblocked.
		if err := s.repo.SetLinkStatus(sys, link.ID, LinkActive); err != nil {
			return Reply{}, err
		}
		link.Status = LinkActive
	}
	if err := s.repo.TouchLink(sys, link.ID, chatID, msg.From.Username, displayName(msg.From)); err != nil {
		s.log.WarnContext(ctx, "telegram: touch link", "error", err)
	}

	switch cmd {
	case "start", "help":
		return replyTo(chatID, helpText), nil
	case "whoami", "me":
		return s.cmdWhoAmI(ctx, link)
	case "org":
		return s.cmdOrg(ctx, link, arg)
	case "notify":
		return s.cmdNotify(ctx, link, arg)
	case "new":
		if err := s.repo.SetConversation(sys, link.ID, nil); err != nil {
			return Reply{}, err
		}
		return replyTo(chatID, "🆕 بدأنا محادثة جديدة. اكتب سؤالك."), nil
	case "unlink":
		if err := s.repo.RevokeLink(sys, link.UserID, link.ID); err != nil {
			return Reply{}, err
		}
		s.log.InfoContext(ctx, "telegram link revoked from chat", "user_id", link.UserID)
		return replyTo(chatID, "تم إلغاء ربط تيليجرام بحسابك في Dawa24. لن يجيب البوت على أسئلتك ولن تصلك إشعارات حتى تربط الحساب من جديد."), nil
	case "":
	default:
		return replyTo(chatID, "لم أتعرف على هذا الأمر.\n\n"+helpText), nil
	}

	if strings.TrimSpace(msg.Text) == "" {
		return replyTo(chatID, "أستقبل حالياً الأسئلة المكتوبة فقط. اكتب سؤالك نصاً."), nil
	}
	return s.answer(ctx, link, msg.Text)
}

// handleLinkCode is the Telegram half of linking.
func (s *Service) handleLinkCode(sys context.Context, msg *Message, code string) (Reply, error) {
	chatID := msg.Chat.ID
	invalid := replyTo(chatID, "هذا الرابط غير صالح أو مستخدم أو منتهي الصلاحية. أنشئ رابطاً جديداً من صفحة الإعدادات في Dawa24.")
	if len(code) > 64 {
		return invalid, nil
	}
	userID, orgID, err := s.repo.ConsumeLinkToken(sys, hashCode(code))
	if errors.Is(err, ErrTokenInvalid) {
		return invalid, nil
	}
	if err != nil {
		return Reply{}, err
	}

	existing, err := s.repo.LiveLinkByTelegramUser(sys, msg.From.ID)
	if err != nil {
		return Reply{}, err
	}
	if existing != nil && existing.UserID == userID && existing.Status != LinkPending {
		if existing.Status == LinkBlocked {
			if err := s.repo.SetLinkStatus(sys, existing.ID, LinkActive); err != nil {
				return Reply{}, err
			}
		}
		return replyTo(chatID, "حساب تيليجرام هذا مربوط بحسابك في Dawa24 بالفعل. اكتب سؤالك أو /help."), nil
	}

	link := &Link{
		UserID:         userID,
		TelegramUserID: msg.From.ID,
		ChatID:         chatID,
		Username:       msg.From.Username,
		DisplayName:    displayName(msg.From),
		ActiveOrgID:    orgID,
	}
	err = s.repo.CreatePendingLink(sys, link, s.now().Add(confirmTTL))
	if errors.Is(err, ErrLinkedElsewhere) {
		return replyTo(chatID, "حساب تيليجرام هذا مربوط بحساب آخر في Dawa24. أرسل /unlink من هنا أولاً ثم أعد المحاولة."), nil
	}
	if err != nil {
		return Reply{}, err
	}
	// Deliberately no account name or email here: whoever opened the code may
	// not be its owner, and learning whose account it was is already a leak.
	return replyTo(chatID,
		"🔗 <b>تم استلام طلب الربط.</b>\n\n"+
			"لإكماله ارجع إلى صفحة الإعدادات في Dawa24 التي أنشأت منها الرابط، وتحقق من أن اسم حساب تيليجرام الظاهر هو حسابك، ثم اضغط «تأكيد الربط».\n\n"+
			"تنتهي المهلة خلال 10 دقائق. إذا لم تطلب هذا الربط فتجاهل الرسالة."), nil
}

// onMembershipChange tracks the user blocking and unblocking the bot.
func (s *Service) onMembershipChange(sys context.Context, m *ChatMemberUpdated) error {
	link, err := s.repo.LiveLinkByTelegramUser(sys, m.From.ID)
	if err != nil || link == nil {
		return err
	}
	switch m.NewChatMember.Status {
	case "kicked":
		if link.Status == LinkActive {
			return s.repo.SetLinkStatus(sys, link.ID, LinkBlocked)
		}
	case "member":
		if link.Status == LinkBlocked {
			return s.repo.SetLinkStatus(sys, link.ID, LinkActive)
		}
	}
	return nil
}

// answer runs one question through the Capsule assistant.
func (s *Service) answer(ctx context.Context, link *Link, question string) (Reply, error) {
	sys := database.AsSystem(ctx)
	chatID := link.ChatID

	actor, refusal, err := s.resolveActor(ctx, link)
	if err != nil {
		return Reply{}, err
	}
	if refusal == "" {
		refusal = approvalRefusal(actor)
	}
	if refusal != "" {
		return replyTo(chatID, refusal), nil
	}
	if !s.assistant.Allowed(actor) {
		return replyTo(chatID, "🔒 استخدام المساعد كبسولة غير مفعّل لدورك في هذه المنشأة. يمكن لمالك المنشأة تفعيله من صفحة الأدوار والصلاحيات."), nil
	}
	if !s.assistant.AllowQuestion(actor.UserID) {
		return replyTo(chatID, "أرسلت أسئلة كثيرة خلال دقيقة. انتظر قليلاً ثم أعد المحاولة."), nil
	}

	acquired, err := s.repo.AcquireBusy(sys, link.ID, s.now().Add(busyFor))
	if err != nil {
		return Reply{}, err
	}
	if !acquired {
		return replyTo(chatID, "⏳ ما زلت أجيب عن سؤالك السابق. سأرد عليه أولاً، ثم أرسل سؤالك الجديد."), nil
	}
	defer func() {
		if err := s.repo.ReleaseBusy(context.WithoutCancel(sys), link.ID); err != nil {
			s.log.WarnContext(ctx, "telegram: release busy", "error", err)
		}
	}()

	// The assistant runs under the caller's tenant and the caller's actor —
	// never under the system context used for this module's own tables. It is
	// detached from n8n's request so a dropped connection still finishes and
	// persists the answer, where the user can read it in the dashboard.
	askCtx := context.WithoutCancel(ctx)
	if actor.OrgID > 0 {
		askCtx = database.WithTenant(askCtx, actor.OrgID)
	}
	askCtx = authctx.WithActor(askCtx, actor)

	var convID int64
	if link.ConversationID != nil {
		convID = *link.ConversationID
	}
	ans := s.assistant.Ask(askCtx, actor, convID, question)
	if ans.ConversationID > 0 && ans.ConversationID != convID {
		id := ans.ConversationID
		if err := s.repo.SetConversation(context.WithoutCancel(sys), link.ID, &id); err != nil {
			s.log.WarnContext(ctx, "telegram: save conversation", "error", err)
		}
	}
	reply := replyTo(chatID, formatAnswer(ans)...)
	for _, p := range ans.Proposals {
		reply.Messages = append(reply.Messages, proposalMessage(chatID, p))
	}
	for _, f := range ans.Files {
		if m, ok := s.documentMessage(chatID, f); ok {
			reply.Messages = append(reply.Messages, m)
		}
	}
	return reply, nil
}

// formatAnswer turns an assistant answer into Telegram messages.
func formatAnswer(ans Answer) []string {
	chunks := RenderMarkdown(ans.Markdown)
	if ans.Failure != "" {
		chunks = append(chunks, "⚠️ "+escape(ans.Failure))
	}
	if links := renderLinks(ans.Links); links != "" {
		if n := len(chunks); n > 0 && units(chunks[n-1])+units(links)+2 <= MaxMessageUnits {
			chunks[n-1] += "\n\n" + links
		} else {
			chunks = append(chunks, links)
		}
	}
	if len(chunks) == 0 {
		chunks = []string{"لم أتمكن من صياغة إجابة لهذا السؤال. جرّب صياغة أوضح."}
	}
	return chunks
}

const maxAnswerLinks = 6

func renderLinks(links []AnswerLink) string {
	var b strings.Builder
	n := 0
	for _, l := range links {
		u := safeHTTPURL(l.URL)
		if u == "" || strings.TrimSpace(l.Title) == "" {
			continue
		}
		if n == 0 {
			b.WriteString("🔗 <b>فتح في Dawa24</b>")
		}
		b.WriteString("\n• <a href=\"" + escape(u) + "\">" + escape(l.Title) + "</a>")
		n++
		if n == maxAnswerLinks {
			break
		}
	}
	return b.String()
}

// parseCommand splits "/cmd@bot arg" into ("cmd", "arg"). A command addressed
// to a different bot is not a command.
func parseCommand(text, bot string) (string, string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	head, arg, _ := strings.Cut(text[1:], " ")
	name, target, hasTarget := strings.Cut(head, "@")
	if hasTarget && bot != "" && !strings.EqualFold(target, bot) {
		return "", ""
	}
	return strings.ToLower(name), strings.TrimSpace(arg)
}

func displayName(u *User) string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

func (s *Service) notLinkedText() string {
	settings := "صفحة الإعدادات"
	if s.cfg.BaseURL != "" {
		settings = `<a href="` + escape(s.cfg.BaseURL+"/settings#telegram") + `">صفحة الإعدادات</a>`
	}
	return "👋 أهلاً بك في بوت <b>Dawa24</b>.\n\n" +
		"لاستخدام المساعد كبسولة واستقبال إشعارات منشأتك، اربط حسابك أولاً:\n" +
		"1. سجّل الدخول إلى Dawa24 وافتح " + settings + ".\n" +
		"2. اضغط «ربط تيليجرام» وافتح الرابط الذي يظهر لك.\n" +
		"3. أكّد الربط من صفحة الإعدادات.\n\n" +
		"لن نطلب منك كلمة المرور هنا أبداً."
}

const helpText = "🤖 <b>مساعد Dawa24 كبسولة على تيليجرام</b>\n\n" +
	"اكتب سؤالك مباشرة عن طلباتك أو مبيعاتك أو محفظتك أو منتجاتك، وسأجيب من بيانات منشأتك حسب صلاحياتك.\n\n" +
	"<b>الأوامر:</b>\n" +
	"/whoami — الحساب والمنشأة والفرع والصلاحيات الحالية\n" +
	"/org — عرض منشآتك وتبديل المنشأة النشطة\n" +
	"/new — بدء محادثة جديدة\n" +
	"/notify — التحكم في إشعارات تيليجرام\n" +
	"/unlink — إلغاء ربط تيليجرام بحسابك\n" +
	"/help — عرض هذه الرسالة"
