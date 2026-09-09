package layouts

import (
	"context"
	"strings"
)

// ResolvedSEO contains pre-computed metadata ready for rendering in document head.
type ResolvedSEO struct {
	Title            string
	MetaDescription  string
	RobotsDirectives string
	CanonicalURL     string
	Keywords         string
	OGTitle          string
	OGDescription    string
	OGImage          string
	TwitterCard      string
	JSONLD           string
}

// ResolveSEO merges site defaults with per-route SEO configuration and locale.
func ResolveSEO(ctx context.Context, defaultTitle, lang string) ResolvedSEO {
	site := GetSiteSettings(ctx)
	seo := GetSEOPage(ctx)

	r := ResolvedSEO{
		Title:            defaultTitle,
		MetaDescription:  site.SiteDescription,
		RobotsDirectives: "index, follow",
		OGTitle:          defaultTitle,
		OGDescription:    site.SiteDescription,
		OGImage:          site.LogoURL,
		TwitterCard:      "summary_large_image",
	}

	if seo != nil {
		if lang == "en" && seo.TitleEN != "" {
			r.Title = seo.TitleEN
		} else if seo.TitleAR != "" {
			r.Title = seo.TitleAR
		}

		if lang == "en" && seo.MetaDescEN != "" {
			r.MetaDescription = seo.MetaDescEN
		} else if seo.MetaDescAR != "" {
			r.MetaDescription = seo.MetaDescAR
		}

		if seo.RobotsDirectives != "" {
			r.RobotsDirectives = seo.RobotsDirectives
		}
		if seo.CanonicalURL != "" {
			r.CanonicalURL = seo.CanonicalURL
		}
		if len(seo.Keywords) > 0 {
			r.Keywords = strings.Join(seo.Keywords, ", ")
		}

		if seo.OGTitle != "" {
			r.OGTitle = seo.OGTitle
		} else {
			r.OGTitle = r.Title
		}

		if seo.OGDesc != "" {
			r.OGDescription = seo.OGDesc
		} else {
			r.OGDescription = r.MetaDescription
		}

		if seo.OGImage != "" {
			r.OGImage = seo.OGImage
		}
		if seo.TwitterCard != "" {
			r.TwitterCard = seo.TwitterCard
		}
		if len(seo.JSONLD) > 2 && string(seo.JSONLD) != "{}" {
			r.JSONLD = string(seo.JSONLD)
		}
	}

	if r.JSONLD == "" && IsCurrentPath(ctx, "/") {
		r.JSONLD = `{"@context":"https://schema.org","@type":"Organization","name":"Dawa24","url":"https://dawa24.com","logo":"https://dawa24.com/static/img/logo.png"}`
	}

	return r
}
