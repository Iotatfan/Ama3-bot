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

func isBotMentioned(message *discordgo.MessageCreate) bool {
	botID := viper.GetString("BOT_ID")
	for _, u := range message.Mentions {
		if u.ID == botID {
			return true
		}
	}
	return false
}

func isReplyToBot(discord *discordgo.Session, message *discordgo.MessageCreate) bool {
	if message.MessageReference == nil || message.MessageReference.MessageID == "" {
		return false
	}

	refID := message.MessageReference.MessageID
	msg, err := discord.ChannelMessage(message.ChannelID, refID)
	if err != nil {
		fmt.Println("reply lookup failed:", err)
		return false
	}

	return msg.Author.ID == viper.GetString("BOT_ID")
}

func ParseGptMessage(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	if message.Author.ID == viper.GetString("BOT_ID") || message.Author.Bot {
		fmt.Println("SKIP")
		return
	}

	if !isBotMentioned(message) && !isReplyToBot(discord, message) {
		return
	}

	if message.ReferencedMessage != nil && message.ReferencedMessage.Author.ID == viper.GetString("BOT_ID") {
		GenerateFollowUpChat(discord, message)
		return
	}

	GenerateNewChat(discord, message, client, ctx)
	return
}

func GenerateNewChat(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input:        responses.ResponseNewParamsInputUnion{OfString: openai.String(message.Content)},
		Model:        openai.ChatModelGPT4_1Nano,
		Instructions: openai.String(viper.GetString("GPT_SYSTEM_PROMPT")),
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

func GenerateFollowUpChat(discord *discordgo.Session, message *discordgo.MessageCreate) {
}
