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

	var resp *responses.Response
	err := h.runModelCall(ctx, "interest", func() error {
		var callErr error
		resp, callErr = h.client.Responses.New(ctx, responses.ResponseNewParams{
			Input: responses.ResponseNewParamsInputUnion{
				OfString: openai.String(interjectionPrompt),
			},
			Model:           openai.ChatModelGPT5_4Mini,
			MaxOutputTokens: openai.Int(10),
			Metadata: shared.Metadata{
				"discord_user_id":    message.Author.ID,
				"discord_guild_id":   message.GuildID,
				"discord_channel_id": message.ChannelID,
			},
		})
		return callErr
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
