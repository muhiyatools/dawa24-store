package telegram

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Notifications on Telegram.
//
// Dawa24 already decides who receives a notification: every producer fans out
// through the in-app feed, checking each member's permission as it goes, and
// the result is a row in notifications.logs per recipient. Telegram does not
// decide that again. It reads the rows that already exist, for users with a
// confirmed link, and adds only what is specific to this channel:
//
//   - the user must still be entitled at delivery time — the grant is
//     re-resolved, because a role revoked between the event and the send must
//     not leak the event to a phone;
//   - the link must be active (not pending, blocked or revoked);
//   - the category must not be muted on Telegram, and offers respect the
//     account-wide offers preference;
//   - only notifications created after the link was confirmed, and recently,
//     are sent, so linking never replays a backlog and an outage never floods.
//
// Every candidate gets a row in telegram.deliveries recording the decision,
// including the decision not to send, so nothing is evaluated twice and "why
// did I not get this" has an answer in the database.

const (
	// notificationWindow is how old a notification may be and still be sent.
	notificationWindow = 2 * time.Hour
	candidateBatch     = 500
	deliveryLease      = 2 * time.Minute
	maxDeliveryTries   = 5
	// MaxClaim bounds one outbox claim.
	MaxClaim = 50
	// processedUpdateRetention bounds the update de-duplication table.
	processedUpdateRetention = 7 * 24 * time.Hour
)

// ClaimOutbox evaluates new notifications and leases due messages to n8n.
func (s *Service) ClaimOutbox(ctx context.Context, limit int) ([]Outgoing, error) {
	sys := database.AsSystem(ctx)
	if limit <= 0 || limit > MaxClaim {
		limit = MaxClaim
	}
	if err := s.materialize(sys); err != nil {
		// Leasing what is already queued is still worth doing.
		s.log.ErrorContext(ctx, "telegram: evaluate notifications", "error", err)
	}
	if err := s.repo.PurgeProcessedUpdates(sys, s.now().Add(-processedUpdateRetention)); err != nil {
		s.log.WarnContext(ctx, "telegram: purge processed updates", "error", err)
	}
	return s.repo.ClaimDeliveries(sys, limit, deliveryLease)
}

func (s *Service) materialize(sys context.Context) error {
	candidates, err := s.repo.NotificationCandidates(sys, s.now().Add(-notificationWindow), candidateBatch)
	if err != nil || len(candidates) == 0 {
		return err
	}

	grants := map[[2]int64]rbac.Grant{}
	offers := map[int64]bool{}
	decisions := make([]Decision, 0, len(candidates))

	for _, c := range candidates {
		d := Decision{LogID: c.LogID, LinkID: c.LinkID, Category: CategoryFor(c.RequiredPermission, c.Title)}
		link := &Link{MutedCategories: c.MutedCategories}

		switch {
		case link.Muted(d.Category):
			d.DropReason = "muted"
		case d.Category == CategoryOffers && !s.offersEnabled(sys, offers, c.UserID):
			d.DropReason = "offers_preference_off"
		default:
			// A notification with no organisation is judged against the chat's
			// active منشأة — the same holding the in-app feed filters it by.
			orgID := c.OrganizationID
			if orgID == 0 {
				orgID = c.LinkActiveOrgID
			}
			key := [2]int64{c.UserID, orgID}
			g, ok := grants[key]
			if !ok {
				if g, err = s.grants.Resolve(sys, c.UserID, orgID); err != nil {
					return err
				}
				grants[key] = g
			}
			if reason := ineligible(g, orgID, c.RequiredPermission); reason != "" {
				d.DropReason = reason
			}
		}
		if d.DropReason == "" {
			d.Text = s.notificationText(c)
		}
		decisions = append(decisions, d)
	}
	return s.repo.RecordDecisions(sys, decisions)
}

// ineligible applies the rule the organisation fan-out applies when it writes
// the feed row (owners, platform staff, or holders of the permission), plus
// the two things only time can change: the account and the membership.
func ineligible(g rbac.Grant, orgID int64, permission string) string {
	platformSide := g.IsStaff || g.IsPlatformOwner
	switch {
	case !g.Active:
		return "account_inactive"
	case orgID > 0 && g.Scope == "":
		return "membership_ended"
	case permission == "":
		return ""
	case platformSide || g.IsOrgOwner || g.Can(permission):
		return ""
	}
	return "permission_revoked"
}

func (s *Service) offersEnabled(sys context.Context, cache map[int64]bool, userID int64) bool {
	if v, ok := cache[userID]; ok {
		return v
	}
	v, err := s.repo.OffersTopicEnabled(sys, userID)
	if err != nil {
		// Unknown preference: do not send marketing.
		s.log.WarnContext(sys, "telegram: read offers preference", "error", err, "user_id", userID)
		v = false
	}
	cache[userID] = v
	return v
}

func (s *Service) notificationText(c Candidate) string {
	var b strings.Builder
	b.WriteString("🔔 <b>" + escape(strings.TrimSpace(c.Title)) + "</b>")
	if body := strings.TrimSpace(c.Body); body != "" {
		b.WriteString("\n" + escape(body))
	}
	if c.OrganizationID > 0 && c.OrganizationID != c.LinkActiveOrgID && c.OrganizationName != "" {
		b.WriteString("\n\n🏢 " + escape(c.OrganizationName))
	}
	if u := safeHTTPURL(s.cfg.BaseURL + "/notifications"); u != "" {
		b.WriteString("\n\n<a href=\"" + escape(u) + "\">عرض الإشعارات في Dawa24</a>")
	}
	text := b.String()
	if units(text) > MaxMessageUnits {
		// A notification body is never this long in practice; if one is, send
		// its title rather than a message Telegram will refuse.
		text = "🔔 <b>" + escape(strings.TrimSpace(c.Title)) + "</b>"
	}
	return text
}

var reRetryAfter = regexp.MustCompile(`retry after (\d+)`)

// ReportDeliveries records what happened to leased messages.
//
// Telegram's error text is the only reliable signal n8n can pass on, so it is
// classified here: a user who blocked the bot or deleted their account stops
// receiving anything until they write again; a flood-control reply is retried
// when Telegram says; a message Telegram cannot parse will never succeed and
// is not retried; anything else backs off and gives up after a few attempts.
func (s *Service) ReportDeliveries(ctx context.Context, results []DeliveryResult) error {
	sys := database.AsSystem(ctx)
	for _, r := range results {
		if r.ID <= 0 {
			continue
		}
		var err error
		msg := strings.ToLower(r.Error)
		switch {
		case r.OK:
			err = s.repo.MarkDelivered(sys, r.ID)
		case strings.Contains(msg, "blocked") || strings.Contains(msg, "deactivated") ||
			strings.Contains(msg, "chat not found") || strings.Contains(msg, "kicked") ||
			strings.Contains(msg, "403"):
			err = s.repo.FailDeliveryAndBlockLink(sys, r.ID, truncate(r.Error, 500))
		case strings.Contains(msg, "can't parse") || strings.Contains(msg, "message is too long"):
			err = s.repo.RetryDelivery(sys, r.ID, truncate(r.Error, 500), 0, 0)
		case reRetryAfter.MatchString(msg):
			secs, _ := strconv.Atoi(reRetryAfter.FindStringSubmatch(msg)[1])
			err = s.repo.RetryDelivery(sys, r.ID, truncate(r.Error, 500), time.Duration(secs+1)*time.Second, maxDeliveryTries)
		default:
			err = s.repo.RetryDelivery(sys, r.ID, truncate(r.Error, 500), 0, maxDeliveryTries)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
