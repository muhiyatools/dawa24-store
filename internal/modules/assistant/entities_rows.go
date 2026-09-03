package assistant

import (
	"strings"
)

// Which read-model rows are records the user can open.
//
// One method per row shape, and nothing else. A row that declares itself here
// becomes clickable everywhere the assistant can return it — in a listing, in a
// detail response, nested inside an aggregate — because the collector finds it
// by type rather than by where it appeared.
//
// Label is the string the model will have written in its prose. That is why it
// is the order NUMBER and not the id: the id is never shown to a model, and the
// number is what the answer says. Where a row has no number, the name is the
// label, which is also what the model writes.

// EntityRef makes a purchase order referenceable.
func (r PurchaseOrderRow) EntityRef() Entity {
	label := strings.TrimSpace(r.Number)
	if label == "" {
		return Entity{}
	}
	return Entity{
		Kind:     EntityOrder,
		ID:       r.ID,
		Label:    label,
		Aliases:  numberAliases(label),
		Title:    "طلب شراء " + label,
		Subtitle: strings.TrimSpace(strings.Join(r.Suppliers, "، ")),
	}
}

// EntityRef makes a supplier shipment referenceable.
func (r SupplyOrderRow) EntityRef() Entity {
	label := strings.TrimSpace(r.Number)
	if label == "" {
		return Entity{}
	}
	return Entity{
		Kind:     EntityShipment,
		ID:       r.ID,
		Label:    label,
		Aliases:  numberAliases(label),
		Title:    "شحنة " + label,
		Subtitle: strings.TrimSpace(r.Buyer),
	}
}

// EntityRef makes a marketplace product referenceable.
func (r MarketProductRow) EntityRef() Entity {
	label := strings.TrimSpace(r.Name)
	if label == "" {
		return Entity{}
	}
	return Entity{
		Kind:     EntityProduct,
		ID:       r.ID,
		Label:    label,
		Title:    label,
		Subtitle: strings.TrimSpace(r.Supplier),
	}
}

// EntityRef makes a vendor's own catalogue item referenceable.
func (r VendorProductRow) EntityRef() Entity {
	label := strings.TrimSpace(r.Name)
	if label == "" {
		return Entity{}
	}
	return Entity{
		Kind:     EntityProduct,
		ID:       r.ID,
		Label:    label,
		Aliases:  skuAliases(r.SKU),
		Title:    label,
		Subtitle: strings.TrimSpace(r.SKU),
	}
}

// EntityRef makes a published offer referenceable.
func (r OfferRow) EntityRef() Entity {
	label := strings.TrimSpace(r.Title)
	if label == "" {
		return Entity{}
	}
	return Entity{
		Kind:  EntityOffer,
		ID:    r.ID,
		Label: label,
		Title: label,
	}
}

// EntityRef makes a registered company referenceable.
func (r OrganizationRow) EntityRef() Entity {
	label := strings.TrimSpace(r.Name)
	if label == "" {
		return Entity{}
	}
	return Entity{
		Kind:     EntityOrganization,
		ID:       r.ID,
		Label:    label,
		Title:    label,
		Subtitle: strings.TrimSpace(r.City),
	}
}

// EntityRef makes a branch referenceable.
func (r BranchRow) EntityRef() Entity {
	label := strings.TrimSpace(r.Name)
	if label == "" {
		return Entity{}
	}
	return Entity{
		Kind:     EntityBranch,
		ID:       r.ID,
		Label:    label,
		Title:    label,
		Subtitle: strings.TrimSpace(r.City),
	}
}

// numberAliases returns the other ways a model may write a reference number.
//
// It writes "#PO-1042" as often as "PO-1042", and Arabic prose routinely drops
// the prefix entirely and says "طلب ١٠٤٢". Only the unambiguous variants are
// offered: the bare numeric tail is included solely when it is long enough that
// matching it cannot collide with an ordinary quantity or price in the same
// sentence.
func numberAliases(label string) []string {
	var out []string
	if !strings.HasPrefix(label, "#") {
		out = append(out, "#"+label)
	}
	if tail := numericTail(label); len(tail) >= 4 && tail != label {
		out = append(out, tail)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// numericTail returns the trailing run of digits in a reference, if any.
func numericTail(s string) string {
	end := len(s)
	i := end
	for i > 0 {
		c := s[i-1]
		if c < '0' || c > '9' {
			break
		}
		i--
	}
	if i == end {
		return ""
	}
	return s[i:end]
}

// skuAliases offers the SKU as a second way to reach a catalogue item. Short
// codes are skipped: a three-character SKU matches too much prose.
func skuAliases(sku string) []string {
	sku = strings.TrimSpace(sku)
	if len(sku) < 4 {
		return nil
	}
	return []string{sku}
}
