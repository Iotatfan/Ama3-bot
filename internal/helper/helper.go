package helper

import (
	"regexp"
	"strings"
)

func MinifyPrompt(prompt string) string {
	spaceRegex := regexp.MustCompile(`\p{Zs}+`)
	minified := spaceRegex.ReplaceAllString(prompt, " ")

	newlineRegex := regexp.MustCompile(`\s*\n\s*`)
	minified = newlineRegex.ReplaceAllString(minified, " ")

	return strings.TrimSpace(minified)
}

func SmartSentenceChunk(text string, limit int) []string {
	var chunks []string

	for len(text) > limit {
		cut := -1

		// Look backwards for sentence or paragraph end.
		for i := limit; i > limit-400 && i > 0; i-- {
			c := text[i]
			if c == '.' || c == '!' || c == '?' || c == '\n' {
				cut = i + 1
				break
			}
		}

		if cut == -1 {
			cut = limit // fallback if no sentence end found
		}

		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}

	if len(text) > 0 {
		chunks = append(chunks, text)
	}

	return chunks
}

func StripBotMention(botID string, content string) string {
	replacer := strings.NewReplacer(
		"<@"+botID+">", "",
		"<@!"+botID+">", "",
	)

	return strings.TrimSpace(strings.Join(strings.Fields(replacer.Replace(content)), " "))
}
