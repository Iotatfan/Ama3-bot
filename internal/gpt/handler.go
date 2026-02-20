package chatGpt

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/spf13/viper"
)

type Post struct {
	Message string `json:"message"`
}

// isBotMentioned returns true if the configured bot ID appears
// in the list of users mentioned in the message.
func isBotMentioned(message *discordgo.MessageCreate) bool {
	botID := viper.GetString("BOT_ID")
	for _, u := range message.Mentions {
		if u.ID == botID {
			return true
		}
	}
	return false
}

// isReplyToBot checks if the incoming message is a reply to a
// message originally sent by the bot itself.
func isReplyToBot(discord *discordgo.Session, message *discordgo.MessageCreate) bool {
	if message.MessageReference == nil || message.MessageReference.MessageID == "" {
		return false
	}

	refID := message.MessageReference.MessageID
	msg, err := discord.ChannelMessage(message.ChannelID, refID)
	if err != nil {
		// unable to fetch the referenced message; assume not a bot reply
		fmt.Println("reply lookup failed:", err)
		return false
	}

	return msg.Author.ID == viper.GetString("BOT_ID")
}

func ParseGptMessage(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	// ignore messages from ourselves or other bots
	if message.Author.ID == viper.GetString("BOT_ID") || message.Author.Bot {
		fmt.Println("SKIP")
		return
	}

	// proceed when either tagged or replying to a bot message
	if !isBotMentioned(message) && !isReplyToBot(discord, message) {
		return
	}

	// TODO: handle the message once we're tagged/replied (generate response, etc.)
	GenerateNewResponse(discord, message, client, ctx)
}

func GenerateNewResponse(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(message.Content)},
		Model: openai.ChatModelGPT4_1Nano,
	})

	if err != nil {
		fmt.Println("error generating response:", err)
		return
	}
	_, err = discord.ChannelMessageSend(message.ChannelID, resp.OutputText())
	if err != nil {
		fmt.Println(err)
	}
}

func GenerateFollowUpResponse(discord *discordgo.Session, message *discordgo.MessageCreate) {
}
