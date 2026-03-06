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
	var post *Post

	if message.Author.ID == config.GetConfig().BotID || message.Author.Bot {
		fmt.Println("SKIP")
		return
	}

	slices := strings.Split(message.Content, "\n")

	for _, slice := range slices {
		post = isTwitterUrl(slice)

		if post != nil && !post.SkipNextCheck {
			post = isInstaUrl(slice)
		}

		if post != nil && post.ShoudlFix {

			_, err := discord.ChannelMessageSendReply(message.ChannelID, post.Message, message.Reference())
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

	log := fmt.Sprintf("%s : %s\n", message.Author, message.Content)
	fmt.Print(log)

	post.Message = ""
	post.ShoudlFix = false
	post.SkipNextCheck = false
}

func assemblePost(matches [][]string, post Post, replaceText string) *Post {
	var builder strings.Builder
	for _, match := range matches {
		builder.WriteString("\n")
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
