package src

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

type Post struct {
	PostUrl       string `json:"post_url"`
	ShoudlFix     bool   `json:"should_url"`
	SkipNextCheck bool   `json:"skip_next_check"`
}

var (
	post *Post
)

func ParseUrl(discord *discordgo.Session, message *discordgo.MessageCreate) {
	if message.GuildID != viper.GetString("SKIP_SERVER") {
		post = isTwitterUrl(message.Content)
	}

	if post != nil && !post.SkipNextCheck {
		post = isInstaUrl(message.Content)
	}

	fmt.Println(message.Author.Username)
	fmt.Println(message.Content)
	fmt.Println(" ")

	if post.ShoudlFix {
		_, err := discord.ChannelMessageSendReply(message.ChannelID, post.PostUrl, message.Reference())
		if err != nil {
			fmt.Println(err)
		}

		discord.ChannelMessageEdit(message.ChannelID, message.ID, message.Content)
		err = discord.ChannelMessageDelete(message.ChannelID, message.ID)
		if err != nil {
			fmt.Println(err)
		}
	}

	// fmt.Println(post.PostUrl)
}

func isTwitterUrl(url string) *Post {
	var post Post

	// TODO: regex
	switch {
	case strings.Contains(url, "https://x.com"):
		post.PostUrl = strings.Replace(url, "https://x.com", "https://fxtwitter.com", 1)
		post.ShoudlFix = true
		post.SkipNextCheck = true
	case strings.Contains(url, "https://twitter.com"):
		post.PostUrl = strings.Replace(url, "https://twitter.com", "https://fxtwitter.com", 1)
		post.ShoudlFix = true
		post.SkipNextCheck = true
	case strings.Contains(url, "https://www.x.com"):
		post.PostUrl = strings.Replace(url, "https://www.x.com", "https://fxtwitter.com", 1)
		post.ShoudlFix = true
		post.SkipNextCheck = true
	case strings.Contains(url, "https://www.twitter.com"):
		post.PostUrl = strings.Replace(url, "https://www.twitter.com", "https://fxtwitter.com", 1)
		post.ShoudlFix = true
		post.SkipNextCheck = true
	default:
		post.PostUrl = url
		post.ShoudlFix = false
		post.SkipNextCheck = false
	}

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
