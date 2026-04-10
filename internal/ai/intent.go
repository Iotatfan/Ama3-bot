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
	IntentNoise             Intent = "noise"
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
	cfg := config.GetConfig()
	intentPrompt := buildIntentPrompt(cfg, message, isReplyFlow, history)

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

	return parseIntentOutput(cleanOutput)
}

func parseIntentOutput(cleanOutput string) Intent {
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
	case "noise":
		return IntentNoise
	default:
		return Unknown
	}
}

func getMessageHistory(discord *discordgo.Session, message *discordgo.MessageCreate, limit int) (string, error) {
	botID := ""
	if cfg := config.GetConfig(); cfg != nil {
		botID = cfg.App.BotID
	}

	pastMessages, err := fetchMessageHistory(discord, message.ChannelID, limit)
	if err != nil {
		fmt.Println("error fetching past messages for interest scoring:", err)
		return message.Content, nil
	}

	return formatMessageHistory(pastMessages, message.ID, botID), nil
}

func fetchMessageHistory(discord *discordgo.Session, channelID string, limit int) ([]*discordgo.Message, error) {
	return discord.ChannelMessages(channelID, limit, "", "", "")
}

func formatMessageHistory(pastMessages []*discordgo.Message, currentMessageID, botID string) string {
	var builder strings.Builder

	// Build history in chronological order, excluding the current message.
	for i := len(pastMessages) - 1; i >= 0; i-- {
		m := pastMessages[i]
		if m.ID == currentMessageID {
			continue
		}

		msgContent := historyMessageContent(m)
		if msgContent == "" {
			continue
		}

		uid, username := historyAuthorLabel(m.Author, botID)
		builder.WriteString(fmt.Sprintf("[UID:%s] %s : %s\n", uid, username, msgContent))
	}

	return builder.String()
}

func historyAuthorLabel(author *discordgo.User, botID string) (string, string) {
	if author == nil {
		return "unknown", "Unknown"
	}

	if author.ID == botID {
		return author.ID, "(Self)"
	}

	username := strings.TrimSpace(author.Username)
	if username == "" {
		username = "Unknown"
	}

	return author.ID, username
}

func historyMessageContent(message *discordgo.Message) string {
	if message == nil {
		return ""
	}

	msgContent := strings.TrimSpace(message.Content)
	if msgContent != "" {
		return msgContent
	}

	if len(message.Embeds) > 0 {
		embedContent := strings.TrimSpace(message.Embeds[0].Description)
		if embedContent != "" {
			return fmt.Sprintf("[EMBED: %s]", embedContent)
		}
	}

	return ""
}

func calculateInterestScore(message *discordgo.MessageCreate, ctx context.Context, client *openai.Client, discord *discordgo.Session) (float32, string) {
	cfg := config.GetConfig()
	combinedContent, _ := getMessageHistory(discord, message, cfg.AI.Interest.PastMessageLimit)
	interjectionPrompt := buildInterestScorePrompt(cfg, message.Content, combinedContent)

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

func buildIntentPrompt(cfg *config.Config, message *discordgo.MessageCreate, isReplyFlow bool, history string) string {
	if cfg == nil {
		return ""
	}

	enrichedContent := getEnrichedContent(message)

	if isReplyFlow {
		targetIsOwner, targetMessage := referencedMessageDetails(message, cfg.App.OwnerID)
		intentPrompt := strings.Replace(cfg.AI.Prompts.IntentReply, "{{.Message}}", enrichedContent, 1)
		intentPrompt = strings.Replace(intentPrompt, "{{.History}}", history, 1)
		intentPrompt = strings.Replace(intentPrompt, "{{.TargetRole}}", strconv.FormatBool(targetIsOwner), 1)
		intentPrompt = strings.Replace(intentPrompt, "{{.TargetMessage}}", targetMessage, 1)
		return intentPrompt
	}

	intentPrompt := strings.Replace(cfg.AI.Prompts.Intent, "{{.Message}}", enrichedContent, 1)
	intentPrompt = strings.Replace(intentPrompt, "{{.History}}", history, 1)
	return intentPrompt
}

func getEnrichedContent(message *discordgo.MessageCreate) string {
	content := message.Content

	var tags []string

	if len(message.Attachments) > 0 {
		tags = append(tags, fmt.Sprintf("[ATTACHMENT_PRESENT: %d file(s)]", len(message.Attachments)))
	}

	if len(message.Embeds) > 0 {
		for _, e := range message.Embeds {
			if e.Image != nil || e.Thumbnail != nil || e.Video != nil {
				tags = append(tags, "[EMBED_PRESENT]")
				break
			}
		}
	}

	if len(tags) > 0 {
		return strings.Join(tags, " ") + " " + content
	}

	return content
}

func referencedMessageDetails(message *discordgo.MessageCreate, ownerID string) (bool, string) {
	if message == nil || message.ReferencedMessage == nil || message.ReferencedMessage.Author == nil {
		return false, ""
	}

	ref := message.ReferencedMessage
	return ref.Author.ID == ownerID, ref.Content
}

func buildInterestScorePrompt(cfg *config.Config, messageContent, history string) string {
	if cfg == nil {
		return ""
	}

	interjectionPrompt := strings.Replace(cfg.AI.Prompts.InterestScore, "{{.Message}}", messageContent, 1)
	interjectionPrompt = strings.Replace(interjectionPrompt, "{{.History}}", history, 1)
	interjectionPrompt = strings.Replace(interjectionPrompt, "{{.OwnerID}}", cfg.App.OwnerID, 1)

	return interjectionPrompt
}

func handlePotentialInterjection(message *discordgo.MessageCreate, ctx context.Context, client *openai.Client, discord *discordgo.Session) (bool, string) {
	score, interjectionMsg := calculateInterestScore(message, ctx, client, discord)

	if score > float32(config.GetConfig().AI.Interest.InterestScoreThreshold) {
		return true, interjectionMsg
	}

	return false, ""
}
