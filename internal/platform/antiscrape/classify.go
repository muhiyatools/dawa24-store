// Package antiscrape refuses automated bulk collection of the public
// marketplace data without getting in a pharmacist's way.
//
// What it defends is specific: /catalog and the other signed-out pages publish
// supplier identity, net supply price, stock and expiry for the whole market.
// That is the commercially valuable part of this platform, and a single
// afternoon with a HTTP client is enough to take all of it.
//
// The design is deliberately three cheap layers rather than one clever one:
//
//  1. classification — refuse callers that announce themselves as a HTTP
//     library or a content harvester, and treat "not obviously a browser" as
//     suspicious rather than as normal;
//  2. budgets — a sliding request allowance per caller per window, so a patient
//     scraper wearing a browser User-Agent still cannot outrun a human;
//  3. penalties — a caller that trips a honeypot is put on the strict path for
//     an hour.
//
// None of this is a bot-detection product and it is not trying to be. It raises
// the cost of taking the catalogue from "trivial" to "deliberate", at the price
// of one string scan and one Redis INCR per protected request. Fingerprinting,
// JavaScript challenges and proof-of-work were considered and rejected: they
// cost more in latency and support tickets than the marginal scraper they stop.
package antiscrape

import (
	"net/http"
	"strings"
)

// Class is what a request looks like it came from.
type Class uint8

const (
	// ClassBrowser is a request that presents as a mainstream browser.
	ClassBrowser Class = iota
	// ClassCrawler is a search or social crawler we want to keep indexing the
	// public pages. Allowed, on a small allowance.
	ClassCrawler
	// ClassUnknown is a request that is neither: no User-Agent, no Accept, or a
	// User-Agent that no browser has ever sent. Served, on a small allowance.
	ClassUnknown
	// ClassAutomation is a request that names itself as a HTTP client, a
	// scraping framework or a content harvester. Refused on data pages.
	ClassAutomation
)

func (c Class) String() string {
	switch c {
	case ClassBrowser:
		return "browser"
	case ClassCrawler:
		return "crawler"
	case ClassAutomation:
		return "automation"
	default:
		return "unknown"
	}
}

// searchCrawlers are the agents that may index the public marketplace.
//
// The User-Agent is not verified by reverse DNS. Doing so costs a DNS lookup on
// the request path to defend against a scraper who could simply have used a
// browser string instead, so the allowance below is small on purpose: claiming
// to be Googlebot buys a lower request budget than claiming to be Chrome.
var searchCrawlers = []string{
	"googlebot", "google-inspectiontool", "adsbot-google", "mediapartners-google",
	"bingbot", "bingpreview", "msnbot",
	"duckduckbot", "applebot", "yandexbot", "baiduspider", "slurp",
	"facebookexternalhit", "twitterbot", "linkedinbot", "pinterestbot",
	"whatsapp", "telegrambot", "discordbot", "embedly", "slackbot",
}

// automationAgents are refused outright on data-heavy public routes.
//
// Three groups, and all three are unwanted here for the same reason: none of
// them is a pharmacist comparing prices.
//
//   - generic HTTP clients and scraping frameworks (curl, requests, scrapy)
//   - headless browser drivers (puppeteer, playwright, selenium)
//   - commercial SEO, AI-training and site-mirroring harvesters
//
// A scraper that changes its User-Agent to Chrome defeats this list in one
// line. It is still worth having: the overwhelming majority of scraping traffic
// never bothers, and the ones that do are then paying the budget of a browser.
var automationAgents = []string{
	// HTTP clients and scripting runtimes
	"curl/", "wget", "libwww-perl", "lwp::", "python-requests", "python-urllib",
	"aiohttp", "httpx/", "urllib", "mechanize", "node-fetch", "axios/",
	"okhttp", "apache-httpclient", "java/", "jakarta", "guzzle", "php/",
	"go-http-client", "ruby", "restsharp", "postmanruntime", "insomnia",
	"httpie", "winhttp", "powershell", "wininet",
	// Scraping and automation frameworks
	"scrapy", "puppeteer", "playwright", "selenium", "webdriver", "phantomjs",
	"headlesschrome", "headless_chrome", "chrome-lighthouse", "cypress",
	// Site mirroring
	"httrack", "webcopier", "webzip", "teleport", "offline explorer",
	"sitesucker", "wget/", "cyotek",
	// Commercial harvesters, SEO crawlers and AI training collectors
	"ahrefsbot", "semrushbot", "mj12bot", "dotbot", "blexbot", "dataforseo",
	"seokicks", "megaindex", "serpstat", "zoominfobot", "petalbot",
	"bytespider", "gptbot", "ccbot", "claudebot", "claude-web", "anthropic-ai",
	"perplexitybot", "amazonbot", "omgili", "diffbot", "imagesiftbot",
	"scrapingbee", "scraperapi", "brightdot", "dataprovider",
	// Scanners; not scrapers, but nothing on a storefront wants them either
	"zgrab", "masscan", "nmap", "nikto", "sqlmap", "wpscan",
}

// Classify decides what a request looks like from its headers alone.
//
// Order matters: the crawler allowlist is consulted first so that a genuine
// Googlebot string is not caught by a substring meant for a harvester.
func Classify(r *http.Request) Class {
	ua := strings.ToLower(strings.TrimSpace(r.Header.Get("User-Agent")))
	if ua == "" {
		return ClassUnknown
	}

	if containsAny(ua, searchCrawlers) {
		return ClassCrawler
	}
	if containsAny(ua, automationAgents) {
		return ClassAutomation
	}

	// Every mainstream browser still sends the "Mozilla/5.0" compatibility
	// prefix, and a self-respecting HTTP client sends its own name instead.
	// Neither is proof, so a miss downgrades to Unknown rather than refusing.
	if !strings.Contains(ua, "mozilla/") {
		return ClassUnknown
	}

	// A browser asks for something. A request with no Accept header at all is a
	// script that forgot to set one.
	if strings.TrimSpace(r.Header.Get("Accept")) == "" {
		return ClassUnknown
	}

	return ClassBrowser
}

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// FromSite reports whether a request plausibly originates from a page of this
// site rather than from a standalone client.
//
// It reads, in order of trustworthiness: the Fetch Metadata headers a modern
// browser sets and a script normally does not, the htmx marker, and finally a
// same-host Referer. Absence of every signal is a "no" — which is the point:
// the JSON search endpoint is meant to serve the compare tool's own page, not
// a caller with a URL.
func FromSite(r *http.Request) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
	case "same-origin", "same-site":
		return true
	case "cross-site", "none":
		// "none" is a direct navigation — someone pasting the endpoint into an
		// address bar, or a client copying a browser's header set.
		return false
	}

	if r.Header.Get("HX-Request") != "" {
		return true
	}

	ref := r.Header.Get("Referer")
	if ref == "" {
		return false
	}
	host := r.Host
	// Compare hosts rather than whole URLs: the scheme differs behind the
	// proxy that terminates TLS.
	return strings.Contains(ref, "://"+host+"/") || strings.HasSuffix(ref, "://"+host)
}
