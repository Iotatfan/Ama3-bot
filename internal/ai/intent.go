package ai

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

type Intent string

const (
	IntentDirect            Intent = "direct"
	IntentReplyToTarget     Intent = "reply_to_target"
	IntentAskAbout          Intent = "ask_about_target"
	IntentValidationRequest Intent = "validation_request"
	IntentActionOnSelf      Intent = "action_on_self"
	IntentInterjection      Intent = "interjection"
	Unknown                 Intent = "unknown"
)

func isBotMentioned(message *discordgo.MessageCreate) bool {
	botID := config.GetConfig().App.BotID
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

	return msg.Author.ID == config.GetConfig().App.BotID
}

func stripBotMention(content string) string {
	botID := config.GetConfig().App.BotID
	replacer := strings.NewReplacer(
		"<@"+botID+">", "",
		"<@!"+botID+">", "",
	)

	return strings.TrimSpace(strings.Join(strings.Fields(replacer.Replace(content)), " "))
}

func determineIntent(message *discordgo.MessageCreate, ctx context.Context, client *openai.Client, isReplyFlow bool, history string) Intent {
	intentPrompt := ""
	if isReplyFlow {
		intentPrompt = strings.Replace(config.GetConfig().AI.Prompts.IntentReply, "{{.Message}}", message.Content, 1)
		intentPrompt = strings.Replace(intentPrompt, "{{.History}}", history, 1)
		intentPrompt = strings.Replace(intentPrompt, "{{.TargetRole}}", strconv.FormatBool(message.ReferencedMessage.Author.ID == config.GetConfig().App.OwnerID), 1)
		intentPrompt = strings.Replace(intentPrompt, "{{.TargetMessage}}", message.ReferencedMessage.Content, 1)
	} else {
		intentPrompt = strings.Replace(config.GetConfig().AI.Prompts.Intent, "{{.Message}}", message.Content, 1)
		intentPrompt = strings.Replace(intentPrompt, "{{.History}}", history, 1)
	}

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(intentPrompt),
		},
		Model: openai.ChatModelGPT5_4Mini,
	})
	if err != nil {
		fmt.Println("error determining intent:", err)
		return IntentDirect
	}

	cleanOutput := strings.ToLower(strings.TrimSpace(resp.OutputText()))
	fmt.Println("Determined intent:", cleanOutput)

	switch cleanOutput {
	case "direct":
		return IntentDirect
	case "reply_to_target":
		return IntentReplyToTarget
	case "ask_about_target":
		return IntentAskAbout
	case "validation_request":
		return IntentValidationRequest
	case "action_on_self":
		return IntentActionOnSelf
	case "interjection":
		return IntentInterjection
	default:
		return Unknown
	}
}

func getMessageHistory(discord *discordgo.Session, message *discordgo.MessageCreate, limit int) (string, error) {
	// Get past messages for context, combine with current message, and feed into interest scoring prompt.
	var builder strings.Builder
	pastMessages, err := discord.ChannelMessages(message.ChannelID, limit, "", "", "")
	if err != nil {
		fmt.Println("error fetching past messages for interest scoring:", err)
		builder.WriteString(message.Content)
		return builder.String(), nil
	}

	// Build history in chronological order, excluding the current message.
	for i := len(pastMessages) - 1; i >= 0; i-- {
		m := pastMessages[i]
		if m.ID == message.ID {
			continue
		}

		if m.Content == "" && m.Embeds != nil && len(m.Embeds) > 0 {
			builder.WriteString(fmt.Sprintf("[UID:%s] %s : [EMBED: %s]\n", m.Author.ID, m.Author.Username, m.Embeds[0].Description))
			continue
		}

		builder.WriteString(fmt.Sprintf("[UID:%s] %s : %s\n", m.Author.ID, m.Author.Username, m.Content))
	}

	return builder.String(), nil
}

func calculateInterestScore(message *discordgo.MessageCreate, ctx context.Context, client *openai.Client, discord *discordgo.Session) (float32, string) {
	combinedContent, _ := getMessageHistory(discord, message, config.GetConfig().AI.Interest.PastMessageLimit)

	interjectionPrompt := strings.Replace(config.GetConfig().AI.Prompts.InterestScore, "{{.Message}}", message.Content, 1)
	interjectionPrompt = strings.Replace(interjectionPrompt, "{{.History}}", combinedContent, 1)
	interjectionPrompt = strings.Replace(interjectionPrompt, "{{.OwnerID}}", config.GetConfig().App.OwnerID, 1)

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(interjectionPrompt),
		},
		Model: openai.ChatModelGPT5_4Mini,
	})
	if err != nil {
		fmt.Println("error calculating interest score:", err)
		return 0, ""
	}

	score, err := strconv.ParseFloat(resp.OutputText(), 32)
	if err != nil {
		fmt.Println("error parsing interest score:", err)
		return 0, ""
	}

	fmt.Println("Calculated interest score:", score)

	return float32(score), combinedContent
}

func handlePotentialInterjection(message *discordgo.MessageCreate, ctx context.Context, client *openai.Client, discord *discordgo.Session) (bool, string) {
	score, interjectionMsg := calculateInterestScore(message, ctx, client, discord)

	if score > float32(config.GetConfig().AI.Interest.InterestScoreThreshold) {
		return true, interjectionMsg
	}

	return false, ""
}
