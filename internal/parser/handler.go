package urlParser

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
)

type Post struct {
	Message       string `json:"message"`
	ShoudlFix     bool   `json:"should_url"`
	SkipNextCheck bool   `json:"skip_next_check"`
}

type Self struct {
	Avatar   string
	Username string
}

func ParseUrl(discord *discordgo.Session, message *discordgo.MessageCreate) {
	if message == nil {
		return
	}

	if message.Author != nil && (message.Author.ID == config.GetConfig().BotID || message.Author.Bot) {
		return
	}

	log := fmt.Sprintf("%s : %s\n", message.Author, message.Content)
	fmt.Print(log)

	slices := strings.Split(message.Content, "\n")

	for _, slice := range slices {
		twitterPost := isTwitterUrl(slice)
		instaPost := isInstaUrl(slice)

		var replies []string
		if twitterPost != nil && twitterPost.ShoudlFix {
			replies = append(replies, twitterPost.Message)
		}
		if instaPost != nil && instaPost.ShoudlFix {
			replies = append(replies, instaPost.Message)
		}

		if len(replies) > 0 {
			replyMessage := strings.Join(replies, "\n")

			_, err := discord.ChannelMessageSendReply(message.ChannelID, replyMessage, message.Reference())
			if err != nil {
				fmt.Println(err)
			}

			_, err = discord.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID:      message.ID,
				Channel: message.ChannelID,
				Flags:   discordgo.MessageFlagsSuppressEmbeds,
			})
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func assemblePost(matches [][]string, post Post, replaceText string) *Post {
	var builder strings.Builder
	for i, match := range matches {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(strings.Replace(match[0], match[1], replaceText, 1))
	}
	post.Message = builder.String()
	post.ShoudlFix = true
	post.SkipNextCheck = true

	return &post
}

func isTwitterUrl(url string) *Post {
	var post Post

	// TODO: store regex else where
	re := regexp.MustCompile(`\bhttps?:\/\/(?:www\.)?(twitter\.com|x\.com)\/[^\/\s]+\/status\/\d+`)
	matches := re.FindAllStringSubmatch(url, -1)

	if len(matches) > 0 {
		return assemblePost(matches, post, config.GetConfig().TwitterReplaceText)
	}

	post.Message = url
	post.ShoudlFix = false
	post.SkipNextCheck = false

	return &post
}

func isInstaUrl(url string) *Post {
	var post Post

	if i := strings.Index(url, "www."); i != -1 {
		url = url[:i] + url[i+4:]
	}

	// TODO: store regex else where
	re := regexp.MustCompile(`\bhttps?:\/\/(?:www\.)?(instagram\.com)\/(?:p|reel)\/[a-zA-Z0-9_-]+`)
	matches := re.FindAllStringSubmatch(url, -1)

	if len(matches) > 0 {
		return assemblePost(matches, post, config.GetConfig().IGReplaceText)
	}

	post.Message = url
	post.ShoudlFix = false
	post.SkipNextCheck = false

	return &post
}
