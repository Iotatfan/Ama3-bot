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
	"github.com/openai/openai-go/v3/shared"
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
	IntentProvocation       Intent = "provocation"
	Unknown                 Intent = "unknown"
)

func isBotMentioned(cfg *config.Config, message *discordgo.MessageCreate) bool {
	if cfg == nil || message == nil {
		return false
	}

	botID := cfg.App.BotID
	for _, u := range message.Mentions {
		if u != nil && u.ID == botID {
			return true
		}
	}

	roleID := cfg.App.RoleID
	for _, u := range message.MentionRoles {
		if u == roleID {
			return true
		}
	}

	return false
}

func (h *AIHandler) isReplyToBot(discord *discordgo.Session, message *discordgo.MessageCreate) bool {
	if message == nil || message.MessageReference == nil || message.MessageReference.MessageID == "" {
		return false
	}

	botID := h.config().App.BotID
	if message.ReferencedMessage != nil {
		return isMessageAuthor(message.ReferencedMessage, botID)
	}
	if discord == nil {
		return false
	}

	refID := message.MessageReference.MessageID
	msg, err := discord.ChannelMessage(message.ChannelID, refID)
	if err != nil {
		fmt.Println("reply lookup failed:", err)
		return false
	}

	return isMessageAuthor(msg, botID)
}

func isMessageAuthor(message *discordgo.Message, authorID string) bool {
	return message != nil && message.Author != nil && message.Author.ID == authorID
}

func (h *AIHandler) determineIntent(message *discordgo.MessageCreate, ctx context.Context, isReplyFlow bool, history string, userSummary string) Intent {
	cfg := h.config()
	intentPrompt := buildIntentPrompt(cfg, message, isReplyFlow, history, userSummary)

	resp, err := h.client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(intentPrompt),
		},
		Model: openai.ChatModelGPT5_4Mini,
		Metadata: shared.Metadata{
			"discord_user_id":    message.Author.ID,
			"discord_guild_id":   message.GuildID,
			"discord_channel_id": message.ChannelID,
		},
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
	case "provocation":
		return IntentProvocation
	default:
		return Unknown
	}
}

func getMessageHistory(discord *discordgo.Session, message *discordgo.MessageCreate, limit int, botID string) (string, error) {
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
		if m == nil {
			continue
		}
		if m.ID == currentMessageID {
			continue
		}

		msgContent := historyMessageContent(m)
		if msgContent == "" {
			continue
		}

		uid, label := historyAuthorLabel(m.Author, botID)
		if label != "" {
			builder.WriteString(fmt.Sprintf("[UID:%s] %s : %s\n", uid, label, msgContent))
		} else {
			builder.WriteString(fmt.Sprintf("[UID:%s] : %s\n", uid, msgContent))
		}
	}

	return builder.String()
}

func historyAuthorLabel(author *discordgo.User, botID string) (uid string, label string) {
	if author == nil {
		return "0", "unknown"
	}

	if author.ID == botID {
		return author.ID, "(Self)"
	}

	return author.ID, author.Username
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

func (h *AIHandler) calculateInterestScore(message *discordgo.MessageCreate, ctx context.Context, discord *discordgo.Session, userSummary string) (float32, string) {
	cfg := h.config()
	combinedContent, _ := getMessageHistory(discord, message, cfg.AI.Interest.PastMessageLimit, cfg.App.BotID)
	interjectionPrompt := buildInterestScorePrompt(cfg, message.Content, combinedContent, userSummary)

	resp, err := h.client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(interjectionPrompt),
		},
		Model: openai.ChatModelGPT5_4Mini,
		Metadata: shared.Metadata{
			"discord_user_id":    message.Author.ID,
			"discord_guild_id":   message.GuildID,
			"discord_channel_id": message.ChannelID,
		},
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

func buildIntentPrompt(cfg *config.Config, message *discordgo.MessageCreate, isReplyFlow bool, history string, userSummary string) string {
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
		intentPrompt = strings.Replace(intentPrompt, "{{.UserSummary}}", userSummary, 1)
		return intentPrompt
	}
	intentPrompt := strings.Replace(cfg.AI.Prompts.Intent, "{{.Message}}", enrichedContent, 1)
	intentPrompt = strings.Replace(intentPrompt, "{{.History}}", history, 1)
	intentPrompt = strings.Replace(intentPrompt, "{{.UserSummary}}", userSummary, 1)

	return intentPrompt
}

func getEnrichedContent(message *discordgo.MessageCreate) string {
	if message == nil {
		return ""
	}
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

func buildInterestScorePrompt(cfg *config.Config, messageContent, history string, userSummary string) string {
	if cfg == nil {
		return ""
	}

	interjectionPrompt := strings.Replace(cfg.AI.Prompts.InterestScore, "{{.Message}}", messageContent, 1)
	interjectionPrompt = strings.Replace(interjectionPrompt, "{{.History}}", history, 1)
	interjectionPrompt = strings.ReplaceAll(interjectionPrompt, "{{.OwnerID}}", cfg.App.OwnerID)
	interjectionPrompt = strings.Replace(interjectionPrompt, "{{.UserSummary}}", userSummary, 1)

	return interjectionPrompt
}

func (h *AIHandler) handlePotentialInterjection(message *discordgo.MessageCreate, ctx context.Context, discord *discordgo.Session, userSummary string) (bool, string) {
	score, interjectionMsg := h.calculateInterestScore(message, ctx, discord, userSummary)

	if score > float32(h.config().AI.Interest.InterestScoreThreshold) {
		return true, interjectionMsg
	}

	return false, ""
}
