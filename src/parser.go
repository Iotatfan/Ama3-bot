package src

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

type Post struct {
	PostUrl   string `json:"post_url"`
	ShoudlFix bool   `json:"should_url"`
}

var (
	post *Post
)

func ParseUrl(discord *discordgo.Session, message *discordgo.MessageCreate) {
	if message.GuildID == viper.GetString("SKIP_SERVER") {
		fmt.Println("skip")
		return
	}

	post = isTwitterUrl(message.Content)
	// post = isInstaUrl(post, message.Content)

	fmt.Println(message.Content)

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

	fmt.Println(post.PostUrl)
}

func isTwitterUrl(url string) *Post {
	var post Post

	switch {
	case strings.Contains(url, "https://x.com"):
		post.PostUrl = strings.Replace(url, "https://x.com", "https://fxtwitter.com", 1)
		post.ShoudlFix = true
	case strings.Contains(url, "https://twitter.com"):
		post.PostUrl = strings.Replace(url, "https://twitter.com", "https://fxtwitter.com", 1)
		post.ShoudlFix = true
	default:
		post.PostUrl = url
		post.ShoudlFix = false
	}

	return &post
}

// func isInstaUrl(url string) (string, bool) {
// 	var newUrl string
// 	var changed bool

// 	switch {
// 	case strings.Contains(url, "https://instagram.com/p"):
// 		newUrl = strings.Replace(url, "https://instagram.com/p", "https://ddinstagram.com/p", 1)
// 		changed = true
// 	case strings.Contains(url, "https://www.instagram.com/p"):
// 		newUrl = strings.Replace(url, "https://www.instagram.com/p", "https://ddinstagram.com/p", 1)
// 		changed = true
// 	default:
// 		newUrl = url
// 		changed = false
// 	}

// 	return newUrl, changed
// }
