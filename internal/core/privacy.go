package core

import (
	"net/url"
	"regexp"
	"strings"
)

var protectedApps = []string{
	"com.1password.1password", "com.agilebits.onepassword7", "1Password",
	"com.bitwarden.desktop", "com.bitwarden.desktop.mas", "com.bitwarden", "Bitwarden",
	"org.keepassxc.keepassxc", "KeePassXC", "com.apple.Passwords", "Passwords", "密码",
	"com.apple.keychainaccess", "Keychain Access", "钥匙串访问",
	"com.apple.SecurityAgent", "com.apple.loginwindow", "io.github.getupandwashits.daytrace",
}

var (
	webURLPattern      = regexp.MustCompile("(?i)https?://[^\\s<>\"'`「」『』《》，。]+")
	privateWindowTitle = regexp.MustCompile(`(?i)private browsing|incognito|inprivate|无痕浏览|隐私浏览|私密浏览`)
)

func protectedApp(bundleID, name string) bool {
	for _, identifier := range protectedApps {
		if strings.EqualFold(identifier, bundleID) || strings.EqualFold(identifier, name) {
			return true
		}
	}
	return false
}

func cleanSample(sample *ActivitySample) {
	if protectedApp(sample.BundleID, sample.App) || vaultTitleRE.MatchString(sample.WindowTitle) ||
		privateWindowTitle.MatchString(sample.WindowTitle) || vaultAddress(sample.URL) {
		*sample = ActivitySample{
			Timestamp: sample.Timestamp, State: "excluded", Accessibility: sample.Accessibility,
			IdleSeconds: sample.IdleSeconds, SampleInterval: sample.SampleInterval, IdleThreshold: sample.IdleThreshold,
		}
		return
	}
	sample.WindowTitle = sanitizeURLs(sample.WindowTitle)
	sample.URL = sanitizeURLs(sample.URL)
	if sample.UIText != nil {
		clean := sanitizeURLs(*sample.UIText)
		sample.UIText = &clean
	}
}

func vaultAddress(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && vaultTitleRE.MatchString(parsed.Hostname())
}

func sanitizeURLs(text string) string {
	return webURLPattern.ReplaceAllStringFunc(text, func(value string) string {
		// Drop the suffix before parsing. Malformed query escapes still must
		// not leave the original suffix in the stored text.
		if index := strings.IndexAny(value, "?#"); index >= 0 {
			value = value[:index]
		}
		parsed, err := url.Parse(value)
		if err != nil {
			return "〔网址已遮蔽〕"
		}
		parsed.User = nil
		return parsed.String()
	})
}
