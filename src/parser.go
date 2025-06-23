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

var (
	post *Post
	self Self
)

func ParseUrl(discord *discordgo.Session, message *discordgo.MessageCreate) {
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
}

func isTwitterUrl(url string) *Post {
	var post Post

	// TODO: store regex else where
	re := regexp.MustCompile(`\bhttps?:\/\/(?:www\.)?(twitter\.com|x\.com)\/[^\/\s]+\/status\/\d+`)
	matches := re.FindAllStringSubmatch(url, -1)

	if len(matches) > 0 {
		var builder strings.Builder
		for index, match := range matches {
			fmt.Println(index, match)

			builder.WriteString("\n")
			builder.WriteString(strings.Replace(match[0], match[1], "fxtwitter.com", 1))
		}
		post.PostUrl = builder.String()
		post.ShoudlFix = true
		post.SkipNextCheck = true

		return &post
	}

	post.PostUrl = url
	post.ShoudlFix = false
	post.SkipNextCheck = false

	return &post
}

func isInstaUrl(url string) *Post {
	var post Post

	// TODO: regex
	switch {
	case strings.Contains(url, "https://instagram.com/p"):
		post.PostUrl = strings.Replace(url, "https://instagram.com/p", "https://ddinstagram.com/p", 1)
		post.ShoudlFix = true
		post.SkipNextCheck = true
	case strings.Contains(url, "https://www.instagram.com/p"):
		post.PostUrl = strings.Replace(url, "https://www.instagram.com/p", "https://ddinstagram.com/p", 1)
		post.ShoudlFix = true
		post.SkipNextCheck = true
	case strings.Contains(url, "https://instagram.com/reel"):
		post.PostUrl = strings.Replace(url, "https://instagram.com/reel", "https://kkinstagram.com/reel", 1)
		post.ShoudlFix = true
		post.SkipNextCheck = true
	case strings.Contains(url, "https://www.instagram.com/reel"):
		post.PostUrl = strings.Replace(url, "https://www.instagram.com/reel", "https://kkinstagram.com/reel", 1)
		post.ShoudlFix = true
		post.SkipNextCheck = true
	default:
		post.PostUrl = url
		post.ShoudlFix = false
		post.SkipNextCheck = false
	}

	return &post
}
