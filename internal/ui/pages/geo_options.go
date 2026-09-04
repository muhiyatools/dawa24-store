package pages

import (
	"strconv"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/components"
)

// Egypt's geography, as combobox options.
//
// Twenty-seven governorates and three hundred and fifty-one cities is small
// enough to ship with the page — about 12 KB — so the pickers filter locally
// and work with no network round trip at all. A search endpoint would be the
// right shape for a list that could not be shipped; this one can.
//
// The city list carries its governorate in Parent, which is what lets the
// second combobox follow the first: selecting a governorate filters the cities
// and clears any city that no longer belongs.

// GovernorateOptions turns the governorate list into combobox options.
func GovernorateOptions(govs []*platformadmin.Governorate, lang string) []components.ComboboxOption {
	out := make([]components.ComboboxOption, 0, len(govs))
	for _, g := range govs {
		if g == nil || !g.IsActive {
			continue
		}
		out = append(out, components.ComboboxOption{
			ID:    strconv.FormatInt(g.ID, 10),
			Label: geoName(g.Name, lang),
			// The other language is searchable but not shown: someone typing
			// "Cairo" should find القاهرة.
			Hint: geoAltName(g.Name, lang),
		})
	}
	return out
}

// CityOptions turns the city list into combobox options, each tagged with the
// governorate it belongs to.
func CityOptions(cities []*platformadmin.City, lang string) []components.ComboboxOption {
	out := make([]components.ComboboxOption, 0, len(cities))
	for _, c := range cities {
		if c == nil || !c.IsActive {
			continue
		}
		parent := ""
		if c.GovernorateID != nil && *c.GovernorateID > 0 {
			parent = strconv.FormatInt(*c.GovernorateID, 10)
		}
		out = append(out, components.ComboboxOption{
			ID:     strconv.FormatInt(c.ID, 10),
			Label:  geoName(c.Name, lang),
			Hint:   geoAltName(c.Name, lang),
			Parent: parent,
		})
	}
	return out
}

// SelectedGeoLabel finds the display name for a previously chosen id, so a
// failed submit re-renders the picker showing what the person had picked rather
// than an empty box.
func SelectedGeoLabel(options []components.ComboboxOption, id string) string {
	if id == "" {
		return ""
	}
	for _, o := range options {
		if o.ID == id {
			return o.Label
		}
	}
	return ""
}

func geoName(name i18n.Text, lang string) string {
	if v := name.Get(i18n.ParseLang(lang)); v != "" {
		return v
	}
	if v := name.Get(i18n.AR); v != "" {
		return v
	}
	return name.Get(i18n.EN)
}

func geoAltName(name i18n.Text, lang string) string {
	primary := geoName(name, lang)
	for _, l := range []i18n.Lang{i18n.AR, i18n.EN} {
		if v := name.Get(l); v != "" && v != primary {
			return v
		}
	}
	return ""
}
