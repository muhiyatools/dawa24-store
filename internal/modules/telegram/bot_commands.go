package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// resolveActor rebuilds the caller for one message, from the database, now.
//
// Nothing about who the user is or what they may do is kept on the link. The
// link names a user and a preferred منشأة; the platform resolver answers the
// rest exactly as it does for a browser request, so a role revoked a second
// ago is revoked here too.
//
// The returned refusal is a user-facing sentence when the caller cannot act at
// all — a suspended account, a membership that has ended, no منشأة chosen.
func (s *Service) resolveActor(ctx context.Context, link *Link) (authctx.Actor, string, error) {
	sys := database.AsSystem(ctx)
	orgID := link.OrgID()

	var memberships []Membership
	if orgID == 0 {
		var err error
		if memberships, err = s.repo.Memberships(sys, link.UserID); err != nil {
			return authctx.Actor{}, "", err
		}
		// One membership is an unambiguous choice; make it and remember it.
		if len(memberships) == 1 {
			orgID = memberships[0].OrgID
			id := orgID
			if err := s.repo.SetActiveOrganization(sys, link.ID, &id); err != nil {
				return authctx.Actor{}, "", err
			}
			link.ActiveOrgID, link.ConversationID = &id, nil
		}
	}

	grant, err := s.grants.Resolve(sys, link.UserID, orgID)
	if err != nil {
		return authctx.Actor{}, "", err
	}
	if !grant.Active {
		return authctx.Actor{}, "🚫 حسابك في Dawa24 غير نشط حالياً، لذلك لا يمكن تنفيذ طلبك.", nil
	}
	platformSide := grant.IsStaff || grant.IsPlatformOwner
	switch {
	case orgID > 0 && grant.Scope == "":
		return authctx.Actor{}, "لم تعد لديك عضوية نشطة في المنشأة المحددة. اختر منشأة أخرى عبر /org", nil
	case orgID == 0 && !platformSide && len(memberships) > 1:
		return authctx.Actor{}, "لديك أكثر من منشأة. اختر المنشأة التي تريد العمل ضمنها عبر /org", nil
	case orgID == 0 && !platformSide:
		return authctx.Actor{}, "لا توجد منشأة نشطة مرتبطة بحسابك في Dawa24.", nil
	}
	return authctx.FromGrant(grant), "", nil
}

// approvalRefusal applies the same rule as authctx.RequireApproved on the
// browser's assistant routes: platform staff pass, everyone else needs an
// approved منشأة.
func approvalRefusal(actor authctx.Actor) string {
	if actor.IsOrgApproved() {
		return ""
	}
	switch actor.OrgStatus {
	case "pending", "under_review":
		return "⏳ منشأتك قيد المراجعة من إدارة المنصة. سيتاح المساعد بعد اعتمادها."
	case "rejected":
		return "🚫 تم رفض طلب اعتماد المنشأة، لذلك لا يمكن استخدام المساعد."
	case "suspended":
		return "🚫 المنشأة موقوفة حالياً، لذلك لا يمكن استخدام المساعد."
	}
	return "🚫 يلزم اعتماد المنشأة لاستخدام المساعد."
}

func (s *Service) cmdWhoAmI(ctx context.Context, link *Link) (Reply, error) {
	sys := database.AsSystem(ctx)
	actor, refusal, err := s.resolveActor(ctx, link)
	if err != nil {
		return Reply{}, err
	}
	if refusal != "" {
		return replyTo(link.ChatID, refusal), nil
	}
	memberships, err := s.repo.Memberships(sys, link.UserID)
	if err != nil {
		return Reply{}, err
	}
	grant, err := s.grants.Resolve(sys, actor.UserID, actor.OrgID)
	if err != nil {
		return Reply{}, err
	}

	var b strings.Builder
	b.WriteString("👤 <b>الحساب:</b> " + escape(orDash(actor.Name)) + "\n")
	current := membershipByOrg(memberships, actor.OrgID)
	switch {
	case current != nil:
		fmt.Fprintf(&b, "🏢 <b>المنشأة:</b> %s — %s\n", escape(current.Name), orgTypeLabel(current.Type))
		b.WriteString("📋 <b>حالة المنشأة:</b> " + orgStatusLabel(current.Status) + "\n")
		branch := "كل فروع المنشأة"
		if current.BranchName != "" {
			branch = current.BranchName + " (صلاحياتك مقصورة على هذا الفرع)"
		}
		b.WriteString("🏬 <b>الفرع:</b> " + escape(branch) + "\n")
	case actor.IsStaff:
		b.WriteString("🏢 <b>النطاق:</b> إدارة المنصة\n")
	}
	b.WriteString("🔐 <b>الدور:</b> " + escape(roleLabel(grant)) + "\n")

	assistantState := "متاح ✅"
	if r := approvalRefusal(actor); r != "" {
		assistantState = "غير متاح — المنشأة غير معتمدة"
	} else if !s.assistant.Allowed(actor) {
		assistantState = "غير مفعّل لدورك 🔒"
	}
	b.WriteString("🤖 <b>المساعد كبسولة:</b> " + assistantState + "\n")
	if len(memberships) > 1 {
		b.WriteString("\nلديك " + strconv.Itoa(len(memberships)) + " منشآت. للتبديل: /org")
	}
	return replyTo(link.ChatID, b.String()), nil
}

// cmdOrg lists the user's organisations, or switches to one of them.
//
// The list is re-read at each call and the choice is an index into it, so the
// only organisations that can be selected are ones the user is an active
// member of right now. Selection still authorises nothing: the next message
// re-resolves membership and permissions for the chosen منشأة.
func (s *Service) cmdOrg(ctx context.Context, link *Link, arg string) (Reply, error) {
	sys := database.AsSystem(ctx)
	memberships, err := s.repo.Memberships(sys, link.UserID)
	if err != nil {
		return Reply{}, err
	}
	platform, err := s.grants.Resolve(sys, link.UserID, 0)
	if err != nil {
		return Reply{}, err
	}
	if !platform.Active {
		return replyTo(link.ChatID, "🚫 حسابك في Dawa24 غير نشط حالياً."), nil
	}
	canPlatform := platform.IsStaff || platform.IsPlatformOwner

	if arg == "" {
		if len(memberships) == 0 && !canPlatform {
			return replyTo(link.ChatID, "لا توجد منشآت نشطة مرتبطة بحسابك."), nil
		}
		var b strings.Builder
		b.WriteString("🏢 <b>اختر المنشأة التي يعمل ضمنها المساعد:</b>\n\n")
		if canPlatform {
			b.WriteString("0. إدارة المنصة" + currentMark(link.OrgID() == 0) + "\n")
		}
		for i, m := range memberships {
			fmt.Fprintf(&b, "%d. %s — %s%s\n", i+1, escape(m.Name), orgTypeLabel(m.Type), currentMark(m.OrgID == link.OrgID()))
		}
		b.WriteString("\nللتبديل أرسل مثلاً: <code>/org 1</code>")
		return replyTo(link.ChatID, b.String()), nil
	}

	n, err := strconv.Atoi(strings.TrimSpace(arg))
	var target *int64
	label := ""
	switch {
	case err == nil && n == 0 && canPlatform:
		label = "إدارة المنصة"
	case err == nil && n >= 1 && n <= len(memberships):
		id := memberships[n-1].OrgID
		target, label = &id, memberships[n-1].Name
	default:
		return replyTo(link.ChatID, "رقم غير صحيح. أرسل /org لعرض القائمة."), nil
	}
	if err := s.repo.SetActiveOrganization(sys, link.ID, target); err != nil {
		return Reply{}, err
	}
	return replyTo(link.ChatID, "✅ أصبحت المنشأة النشطة: <b>"+escape(label)+"</b>\nبدأت محادثة جديدة ضمن هذه المنشأة."), nil
}

// cmdNotify shows or changes which notification categories reach Telegram.
func (s *Service) cmdNotify(ctx context.Context, link *Link, arg string) (Reply, error) {
	sys := database.AsSystem(ctx)
	fields := strings.Fields(strings.ToLower(arg))
	if len(fields) == 2 && (fields[1] == "on" || fields[1] == "off") {
		muted := map[string]bool{}
		for _, c := range link.MutedCategories {
			muted[c] = true
		}
		off := fields[1] == "off"
		if fields[0] == "all" {
			for _, c := range Categories {
				muted[string(c)] = off
			}
		} else if c, ok := ParseCategory(fields[0]); ok {
			muted[string(c)] = off
		} else {
			return replyTo(link.ChatID, "فئة غير معروفة. أرسل /notify لعرض الفئات."), nil
		}
		var next []string
		for _, c := range Categories {
			if muted[string(c)] {
				next = append(next, string(c))
			}
		}
		if err := s.repo.SetMutedCategories(sys, link.ID, next); err != nil {
			return Reply{}, err
		}
		link.MutedCategories = next
	} else if arg != "" {
		return replyTo(link.ChatID, "الصيغة: <code>/notify orders off</code> أو <code>/notify all on</code>"), nil
	}

	offers, err := s.repo.OffersTopicEnabled(sys, link.UserID)
	if err != nil {
		return Reply{}, err
	}
	var b strings.Builder
	b.WriteString("🔔 <b>إشعارات تيليجرام</b>\n")
	b.WriteString("تصلك فقط الإشعارات التي تسمح بها صلاحياتك في Dawa24.\n\n")
	for _, c := range Categories {
		state, toggle := "✅", "off"
		if link.Muted(c) {
			state, toggle = "🔕", "on"
		}
		fmt.Fprintf(&b, "%s %s — <code>/notify %s %s</code>\n", state, c.Label(), c, toggle)
	}
	if !offers {
		b.WriteString("\nℹ️ العروض الترويجية متوقفة من تفضيلات حسابك في Dawa24، ولن تصل هنا أيضاً.")
	}
	return replyTo(link.ChatID, b.String()), nil
}

func membershipByOrg(ms []Membership, orgID int64) *Membership {
	for i := range ms {
		if ms[i].OrgID == orgID {
			return &ms[i]
		}
	}
	return nil
}

func currentMark(current bool) string {
	if current {
		return " ✅ (الحالية)"
	}
	return ""
}

func roleLabel(g rbac.Grant) string {
	switch {
	case g.IsPlatformOwner:
		return "مالك المنصة"
	case g.IsStaff:
		return "فريق إدارة المنصة"
	case g.IsOrgOwner:
		return "مالك المنشأة"
	case g.MemberRoleName != "":
		return g.MemberRoleName
	}
	return "عضو"
}

func orgTypeLabel(t string) string {
	switch rbac.NormalizeOrgType(t) {
	case "vendor":
		return "مورّد"
	case "customer":
		return "صيدلية"
	}
	return "منشأة"
}

func orgStatusLabel(s string) string {
	switch s {
	case "approved", "active", "verified":
		return "معتمدة ✅"
	case "pending", "under_review":
		return "قيد المراجعة ⏳"
	case "suspended":
		return "موقوفة 🚫"
	case "rejected":
		return "مرفوضة 🚫"
	}
	return escape(s)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
