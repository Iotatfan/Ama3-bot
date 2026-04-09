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
)

type Post struct {
	Message       string
	ShouldFix     bool
	SkipNextCheck bool
}

func ParseUrl(discord *discordgo.Session, message *discordgo.MessageCreate) {
	if !config.GetConfig().Platform.Replacements.Enabled {
		return
	}

	if message == nil {
		return
	}

	if message.Author != nil && (message.Author.ID == config.GetConfig().App.BotID || message.Author.Bot) {
		return
	}

	if message.GuildID != "" && contains(config.GetConfig().Platform.BlacklistGuilds, message.GuildID) {
		return
	}

	fmt.Printf("URL replace check user_id=%s channel_id=%s len=%d\n", message.Author.ID, message.ChannelID, len(message.Content))

	lines := strings.Split(message.Content, "\n")
	var allReplies []string

	for _, line := range lines {
		// Check Twitter
		if twitterPost := findAndReplace(line, twitterRegex, config.GetConfig().Platform.Replacements.Twitter); twitterPost.ShouldFix {
			allReplies = append(allReplies, twitterPost.Message)
		}
		// Check Instagram
		if instaPost := findAndReplace(line, instaRegex, config.GetConfig().Platform.Replacements.Instagram); instaPost.ShouldFix {
			allReplies = append(allReplies, instaPost.Message)
		}
	}

	if len(allReplies) > 0 {
		replyMessage := strings.Join(allReplies, "\n")

		_, err := discord.ChannelMessageSendReply(message.ChannelID, replyMessage, message.Reference())
		if err != nil {
			fmt.Println("Error sending reply:", err)
		}

		// Suppress original embeds
		_, err = discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID:      message.ID,
			Channel: message.ChannelID,
			Flags:   discordgo.MessageFlagsSuppressEmbeds,
		})
		if err != nil {
			fmt.Println("Error suppressing embeds:", err)
		}
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func findAndReplace(input string, re *regexp.Regexp, replacement string) Post {
	matches := re.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return Post{Message: input, ShouldFix: false}
	}

	var builder strings.Builder
	for i, match := range matches {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(strings.Replace(match[0], match[1], replacement, 1))
	}

	return Post{
		Message:   builder.String(),
		ShouldFix: true,
	}
}
