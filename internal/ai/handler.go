package ai

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/helper"
)

func (h *AIHandler) ParseMessage(discord *discordgo.Session, message *discordgo.MessageCreate, ctx context.Context) {
	if message == nil {
		return
	}

	if message.Author == nil {
		return
	}

	cfg := h.config()
	client := h.client
	if client == nil {
		fmt.Println("OpenAI client is not configured")
		return
	}

	if message.Author.ID == cfg.App.BotID || message.Author.Bot {
		return
	}

	if message.Content == "" && len(message.Attachments) == 0 && len(message.Embeds) == 0 {
		return
	}

	fmt.Printf("Received message user_id=%s channel_id=%s len=%d\n", message.Author.ID, message.ChannelID, len(message.Content))

	if cfg.AI.Summary.Enabled && message.Content != "" {
		if msgs, should := h.userMessageCounter.AddMessageAndCheckSummary(message.Author.ID, message.Content, cfg.AI.Summary.MessageThreshold); should {
			go h.updateUserSummary(message.Author.ID, message.Author.Username, msgs, message.GuildID, message.ChannelID, context.Background())
		}
	}

	userSummary, _ := h.getUserSummary(message.Author.ID)

	if message.GuildID != "" && !isBotMentioned(cfg, message) && !h.isReplyToBot(discord, message) {
		if cfg.AI.Interest.EnableInterestDetection {
			if !h.isNotCooldown(message.ChannelID) {
				fmt.Println("Channel is in cooldown, skipping interest check")
				return
			}

			shouldHandle, history := h.handlePotentialInterjection(message, ctx, discord, userSummary)
			if shouldHandle {
				h.updateChannelActivity(message.ChannelID)

				fmt.Println("Message is not directed at bot and has high interest score, generating interjection response...")
				message.Content = helper.StripBotMention(cfg.App.BotID, message.Content)

				h.generateNewChat(discord, message, ctx, IntentInterjection, history, userSummary, "")
				return
			}
			fmt.Println("Message is not directed at bot and has low interest score, skipping...")
			return
		}
		return
	}

	if !h.allowDirectFlow(message.Author.ID, message.ChannelID) {
		fmt.Printf("Direct flow rate-limited user_id=%s channel_id=%s\n", message.Author.ID, message.ChannelID)
		return
	}

	history, _ := getMessageHistory(discord, message, cfg.AI.Interest.PastMessageLimit, cfg.App.BotID)

	message.Content = helper.StripBotMention(cfg.App.BotID, message.Content)
	intent := h.determineIntent(message, ctx, message.ReferencedMessage != nil, history, userSummary)
	targetSummary := h.getMentionedTargetSummary(message, intent)

	if message.MessageReference != nil && isMessageAuthor(message.ReferencedMessage, cfg.App.BotID) {
		convID, ok := h.conversationMap.GetConversationByRef(message.MessageReference.MessageID)
		if ok {
			fmt.Println("Found conversation ID:", convID)
			h.generateFollowUpChat(discord, message, ctx, intent, history, userSummary, targetSummary)
			return
		}
	}

	fmt.Println("Could not find conversation for reference message")
	fmt.Println("Generating new chat...")
	h.generateNewChat(discord, message, ctx, intent, history, userSummary, targetSummary)
}

func (h *AIHandler) updateUserSummary(uid string, username string, msgs []string, guildID string, channelID string, ctx context.Context) {
	userSummary, err := h.getUserSummary(uid)
	if err != nil {
		fmt.Println("Error fetching user summary:", err)
		return
	}

	updatedUserSummary, err := h.GenerateUserSummary(uid, username, userSummary, msgs, guildID, channelID, ctx)
	if err != nil {
		fmt.Println("Error generating updated user summary:", err)
		return
	}

	h.userMessageCounter.UpdateSummary(uid, updatedUserSummary)

	if h.userRepo != nil {
		err := h.userRepo.UpsertUserSummary(uid, updatedUserSummary)
		if err != nil {
			fmt.Println("Error saving user summary to db:", err)
		}
	}
}

func (h *AIHandler) getUserSummary(uid string) (string, error) {
	if !h.config().AI.Summary.Enabled {
		return "", nil
	}

	h.userMessageCounter.mu.RLock()
	stats, exists := h.userMessageCounter.counters[uid]
	h.userMessageCounter.mu.RUnlock()

	if exists && stats.Summary != "" {
		return stats.Summary, nil
	}

	if h.userRepo == nil {
		return "", nil
	}

	summary, err := h.userRepo.GetUserSummary(uid)
	if err != nil {
		return "", err
	}

	if summary == "" {
		return "", nil
	}

	h.userMessageCounter.UpdateSummary(uid, summary)
	return helper.MinifyPrompt(summary), nil
}

func (h *AIHandler) getMentionedTargetSummary(message *discordgo.MessageCreate, intent Intent) string {
	if intent != IntentAskAbout {
		return ""
	}

	targetUID := mentionedTargetUID(message, h.config().App.BotID)
	if targetUID == "" {
		return ""
	}

	targetSummary, err := h.getUserSummary(targetUID)
	if err != nil {
		fmt.Printf("Error fetching target user summary uid=%s: %v\n", targetUID, err)
		return ""
	}

	return targetSummary
}

func mentionedTargetUID(message *discordgo.MessageCreate, botID string) string {
	if message == nil {
		return ""
	}

	for _, mention := range message.Mentions {
		if mention == nil || mention.ID == "" || mention.ID == botID {
			continue
		}

		return mention.ID
	}

	return ""
}
