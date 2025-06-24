package src

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

type Post struct {
	PostUrl       string `json:"post_url"`
	ShoudlFix     bool   `json:"should_url"`
	SkipNextCheck bool   `json:"skip_next_check"`
}

type Self struct {
	Avatar   string
	Username string
}

func ParseUrl(discord *discordgo.Session, message *discordgo.MessageCreate) {
	var post *Post

	if message.Author.ID == viper.GetString("BOT_ID") || message.Author.Bot {
		fmt.Println("SKIP")
		return
	}

	if message.GuildID != viper.GetString("SKIP_SERVER") {
		post = isTwitterUrl(message.Content)
	}

	if post != nil && !post.SkipNextCheck {
		post = isInstaUrl(message.Content)
	}

	fmt.Println(message.Author.Username)
	fmt.Println(message.Content)
	fmt.Println(" ")

	if post != nil && post.ShoudlFix {
		_, err := discord.ChannelMessageSend(message.ChannelID, post.PostUrl)
		if err != nil {
			fmt.Println(err)
		}

		err = discord.ChannelMessageDelete(message.ChannelID, message.ID)
		if err != nil {
			fmt.Println(err)
		}
	}

	post.PostUrl = ""
	post.ShoudlFix = false
	post.SkipNextCheck = false
}

func assemblePost(matches [][]string, post Post, replaceText string) *Post {
	var builder strings.Builder
	for index, match := range matches {
		fmt.Println(index, match)

		builder.WriteString("\n")
		builder.WriteString(strings.Replace(match[0], match[1], replaceText, 1))
	}
	post.PostUrl = builder.String()
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
		return assemblePost(matches, post, "fxtwitter.com")
	}

	post.PostUrl = url
	post.ShoudlFix = false
	post.SkipNextCheck = false

	return &post
}

func isInstaUrl(url string) *Post {
	var post Post

	// TODO: store regex else where
	re := regexp.MustCompile(`/\bhttps?:\/\/(?:www\.)?(instagram\.com\/(?:p|reel))\/[a-zA-Z0-9_-]+`)
	matches := re.FindAllStringSubmatch(url, -1)

	if len(matches) > 0 {
		return assemblePost(matches, post, "ddinstagram.com")
	}

	post.PostUrl = url
	post.ShoudlFix = false
	post.SkipNextCheck = false

	return &post
}
