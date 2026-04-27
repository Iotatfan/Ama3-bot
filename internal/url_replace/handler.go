package url_replace

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
)

var (
	twitterRegex = regexp.MustCompile(`\bhttps?:\/\/(?:www\.)?(twitter\.com|x\.com)\/[^\/\s]+\/status\/\d+`)
	instaRegex   = regexp.MustCompile(`\bhttps?:\/\/(?:www\.)?(instagram\.com)\/(?:p|reel)\/[a-zA-Z0-9_-]+`)

	defaultURLReplaceHandler = NewURLReplaceHandler()
)

type urlReplacementRule struct {
	name        string
	pattern     *regexp.Regexp
	replacement func(cfg *config.Config) string
}

type URLReplaceHandler struct {
	getConfig func() *config.Config
	rules     []urlReplacementRule
}

func ParseUrl(discord *discordgo.Session, message *discordgo.MessageCreate) {
	defaultURLReplaceHandler.ParseUrl(discord, message)
}

func NewURLReplaceHandler() *URLReplaceHandler {
	return NewURLReplaceHandlerWithConfig(config.GetConfig)
}

func NewURLReplaceHandlerWithConfig(getConfig func() *config.Config) *URLReplaceHandler {
	if getConfig == nil {
		getConfig = config.GetConfig
	}

	h := &URLReplaceHandler{
		getConfig: getConfig,
	}

	h.rules = []urlReplacementRule{
		{
			name:    "twitter",
			pattern: twitterRegex,
			replacement: func(cfg *config.Config) string {
				if cfg == nil {
					return ""
				}
				return cfg.Platform.Replacements.Twitter
			},
		},
		{
			name:    "instagram",
			pattern: instaRegex,
			replacement: func(cfg *config.Config) string {
				if cfg == nil {
					return ""
				}
				return cfg.Platform.Replacements.Instagram
			},
		},
	}

	return h
}

func (h *URLReplaceHandler) ParseUrl(discord *discordgo.Session, message *discordgo.MessageCreate) {
	cfg := h.getConfig()
	if !h.shouldProcessMessage(cfg, message) {
		return
	}

	authorID := "unknown"
	if message.Author != nil {
		authorID = message.Author.ID
	}
	fmt.Printf("URL replace check user_id=%s channel_id=%s len=%d\n", authorID, message.ChannelID, len(message.Content))

	replies := h.buildReplacementReplies(message.Content, cfg)
	if len(replies) == 0 {
		return
	}

	replyMessage := strings.Join(replies, "\n")
	if _, err := discord.ChannelMessageSendReply(message.ChannelID, replyMessage, message.Reference()); err != nil {
		fmt.Println("Error sending reply:", err)
	}

	if err := h.suppressEmbeds(discord, message); err != nil {
		fmt.Println("Error suppressing embeds:", err)
	}
}

func (h *URLReplaceHandler) shouldProcessMessage(cfg *config.Config, message *discordgo.MessageCreate) bool {
	if cfg == nil || message == nil {
		return false
	}

	if !cfg.Platform.Replacements.Enabled {
		return false
	}

	if message.Author != nil && (message.Author.ID == cfg.App.BotID || message.Author.Bot) {
		return false
	}

	if message.GuildID != "" && !contains(cfg.Platform.WhitelistGuilds, message.GuildID) {
		return false
	}

	return true
}

func (h *URLReplaceHandler) buildReplacementReplies(content string, cfg *config.Config) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	lines := strings.Split(content, "\n")
	replies := make([]string, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		for _, rule := range h.rules {
			replies = append(replies, h.applyRule(line, rule, cfg)...)
		}
	}

	return replies
}

func (h *URLReplaceHandler) applyRule(input string, rule urlReplacementRule, cfg *config.Config) []string {
	if rule.pattern == nil || rule.replacement == nil {
		return nil
	}

	replacementHost := strings.TrimSpace(rule.replacement(cfg))
	if replacementHost == "" {
		fmt.Printf("URL replacement skipped rule=%s because replacement host is empty\n", rule.name)
		return nil
	}

	matches := rule.pattern.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return nil
	}

	fixedURLs := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		fixedURLs = append(fixedURLs, strings.Replace(match[0], match[1], replacementHost, 1))
	}

	return fixedURLs
}

func (h *URLReplaceHandler) suppressEmbeds(discord *discordgo.Session, message *discordgo.MessageCreate) error {
	_, err := discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:      message.ID,
		Channel: message.ChannelID,
		Flags:   discordgo.MessageFlagsSuppressEmbeds,
	})

	return err
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
