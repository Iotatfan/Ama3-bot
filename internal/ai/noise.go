package ai

import (
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var punctuationOnlyRE = regexp.MustCompile(`^[^[:alnum:]]+$`)

func isNoiseMessage(message *discordgo.MessageCreate) bool {
	if message == nil || message.Message == nil {
		return true
	}
	if len(message.Attachments) > 0 || len(message.Embeds) > 0 {
		return false
	}
	text := strings.TrimSpace(strings.ToLower(message.Content))
	if text == "" || punctuationOnlyRE.MatchString(text) {
		return true
	}
	tokens := memoryTokenRE.FindAllString(text, -1)
	if len(tokens) == 0 || len(tokens) > 5 {
		return false
	}
	for _, token := range tokens {
		if !memoryNoise[token] {
			return false
		}
	}
	return true
}
