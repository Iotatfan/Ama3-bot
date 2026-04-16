package ai

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
	"github.com/openai/openai-go/v3"
)

func ParseMessage(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	defaultAIHandler.ParseMessage(discord, message, client, ctx)
}

func (h *AIHandler) ParseMessage(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	if message == nil {
		return
	}

	if message.Author == nil {
		return
	}

	if message.Author.ID == config.GetConfig().App.BotID || message.Author.Bot {
		return
	}

	if message.Content == "" && len(message.Attachments) == 0 && len(message.Embeds) == 0 {
		return
	}

	fmt.Printf("Received message user_id=%s channel_id=%s len=%d\n", message.Author.ID, message.ChannelID, len(message.Content))

	if config.GetConfig().AI.Summary.Enabled && message.Content != "" {
		if msgs, should := h.userMessageCounter.AddMessageAndCheckSummary(message.Author.ID, message.Content, config.GetConfig().AI.Summary.MessageThreshold); should {
			go h.updateUserSummary(message.Author.ID, message.Author.Username, msgs, client, context.Background())
		}
	}

	if message.GuildID != "" {
		perms, err := discord.UserChannelPermissions(config.GetConfig().App.BotID, message.ChannelID)
		if err != nil {
			fmt.Println("Error checking permissions:", err)
			return
		}

		if perms&discordgo.PermissionSendMessages == 0 {
			fmt.Printf("Missing permission to send messages in channel_id=%s\n", message.ChannelID)

			dmChannel, err := discord.UserChannelCreate(message.Author.ID)
			if err != nil {
				fmt.Println("Failed to create DM channel:", err)
				return
			}

			_, err = discord.ChannelMessageSend(dmChannel.ID, fmt.Sprintf("I don't have permission to reply in <#%s>.", message.ChannelID))
			if err != nil {
				fmt.Println("Failed to send DM message:", err)
			}

			return
		}
	}

	userSummary, _ := h.getUserSummary(message.Author.ID)

	if message.GuildID != "" && !isBotMentioned(message) && !isReplyToBot(discord, message) {
		if config.GetConfig().AI.Interest.EnableInterestDetection {
			if !h.isNotCooldown(message.ChannelID) {
				fmt.Println("Channel is in cooldown, skipping interest check")
				return
			}

			shouldHandle, history := handlePotentialInterjection(message, ctx, client, discord, userSummary)
			if shouldHandle {
				h.updateChannelActivity(message.ChannelID)

				fmt.Println("Message is not directed at bot and has high interest score, generating interjection response...")
				message.Content = stripBotMention(message.Content)

				h.generateNewChat(discord, message, client, ctx, IntentInterjection, history, userSummary)
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

	history, _ := getMessageHistory(discord, message, config.GetConfig().AI.Interest.PastMessageLimit)

	message.Content = stripBotMention(message.Content)
	intent := determineIntent(message, ctx, client, message.ReferencedMessage != nil, history, userSummary)

	if message.MessageReference != nil && message.ReferencedMessage != nil && message.ReferencedMessage.Author.ID == config.GetConfig().App.BotID {
		convID, ok := h.conversationMap.GetConversationByRef(message.MessageReference.MessageID)
		if ok {
			fmt.Println("Found conversation ID:", convID)
			h.generateFollowUpChat(discord, message, client, ctx, intent, history, userSummary)
			return
		}
	}

	fmt.Println("Could not find conversation for reference message")
	fmt.Println("Generating new chat...")
	h.generateNewChat(discord, message, client, ctx, intent, history, userSummary)
}

func (h *AIHandler) updateUserSummary(uid string, username string, msgs []string, client *openai.Client, ctx context.Context) {
	userSummary, err := h.getUserSummary(uid)
	if err != nil {
		fmt.Println("Error fetching user summary:", err)
		return
	}

	updatedUserSummary, err := h.GenerateUserSummary(username, userSummary, msgs, client, ctx)
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
	if !config.GetConfig().AI.Summary.Enabled {
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
	return summary, nil
}
