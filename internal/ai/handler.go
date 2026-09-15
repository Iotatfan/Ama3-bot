package ai

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
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
	if message.GuildID != "" && !isBotMentioned(cfg, message) && !h.isReplyToBot(discord, message) {
		if cfg.AI.Interest.EnableInterestDetection {
			userSummary, _ := h.getUserSummary(message.Author.ID)
			if !h.isNotCooldown(message.ChannelID) {
				fmt.Println("Channel is in cooldown, skipping interest check")
				return
			}

			shouldHandle, history := h.handlePotentialInterjection(message, ctx, discord, userSummary)
			if shouldHandle {
				h.updateChannelActivity(message.ChannelID)

				fmt.Println("Message is not directed at bot and has high interest score, generating interjection response...")
				message.Content = helper.StripBotMention(cfg.App.BotID, message.Content)

				h.generateNewChat(discord, message, ctx, false, history, userSummary, "")
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

	if cfg.AI.Summary.Enabled && message.Content != "" {
		if msgs, should := h.userMessageCounter.AddMessageAndCheckSummary(message.Author.ID, message.Content, cfg.AI.Summary.MessageThreshold); should {
			go h.updateUserSummary(message.Author.ID, message.Author.Username, msgs, message.GuildID, message.ChannelID, context.Background())
		}
	}

	userSummary, _ := h.getUserSummary(message.Author.ID)
	h.queueMemory(message, userSummary)

	history, _ := getMessageHistory(discord, message, cfg.AI.Interest.PastMessageLimit, cfg.App.BotID)
	memories, selfMemories, memErr := h.retrieveMemories(ctx, message, history, userSummary)
	if memErr != nil {
		fmt.Println("long-term memory retrieval failed:", memErr)
	}
	longMemory := formatMemories(memories, cfg.AI.Memory.MaxInjectedCharacters)
	selfMemory := formatSelfMemories(selfMemories, cfg.AI.Memory.SelfMaxInjectedCharacters)
	fmt.Printf("memory prompt injection user_id=%s retrieved=%d injected=%t injected_characters=%d\n", message.Author.ID, len(memories), longMemory != "", len(longMemory))
	message.Content = helper.StripBotMention(cfg.App.BotID, message.Content)
	noise := isNoiseMessage(message)

	if message.MessageReference != nil && isMessageAuthor(message.ReferencedMessage, cfg.App.BotID) {
		convID, ok := h.conversationMap.GetConversationByRef(message.MessageReference.MessageID)
		if ok {
			fmt.Println("Found conversation ID:", convID)
			h.generateFollowUpChat(discord, message, ctx, noise, history, userSummary, longMemory, selfMemory)
			return
		}
	}

	fmt.Println("Could not find conversation for reference message")
	fmt.Println("Generating new chat...")
	h.generateNewChat(discord, message, ctx, noise, history, userSummary, longMemory, selfMemory)
}

func isWhitelistedGuild(cfg *config.Config, guildID string) bool {
	if cfg == nil || guildID == "" {
		return false
	}
	for _, allowed := range cfg.Platform.WhitelistGuilds {
		if allowed == guildID {
			return true
		}
	}
	return false
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
