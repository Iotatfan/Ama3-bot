package ai

import (
	"context"
	"fmt"
	"math/rand"

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

	if message.Author.ID == config.GetConfig().App.BotID || message.Author.Bot {
		return
	}

	if message.Content == "" && len(message.Attachments) == 0 && len(message.Embeds) == 0 {
		return
	}

	fmt.Printf("Received message user_id=%s channel_id=%s len=%d\n", message.Author.ID, message.ChannelID, len(message.Content))

	if !isBotMentioned(message) && !isReplyToBot(discord, message) && config.GetConfig().AI.Interest.EnableInterestDetection {
		if !h.isNotCooldown(message.ChannelID) {
			fmt.Println("Channel is in cooldown, skipping interest check")
			return
		}

		shouldHandle, history := handlePotentialInterjection(message, ctx, client, discord)
		if shouldHandle {
			h.updateChannelActivity(message.ChannelID)

			fmt.Println("Message is not directed at bot and has high interest score, generating interjection response...")
			message.Content = stripBotMention(message.Content)

			h.generateNewChat(discord, message, client, ctx, IntentInterjection, history)
			return
		}
		fmt.Println("Message is not directed at bot and has low interest score, skipping...")
		return
	}

	if !h.allowDirectFlow(message.Author.ID, message.ChannelID) {
		fmt.Printf("Direct flow rate-limited user_id=%s channel_id=%s\n", message.Author.ID, message.ChannelID)
		return
	}

	history, _ := getMessageHistory(discord, message, config.GetConfig().AI.Interest.PastMessageLimit)

	message.Content = stripBotMention(message.Content)
	intent := determineIntent(message, ctx, client, message.ReferencedMessage != nil, history)

	if intent == IntentNoise {
		reactToNoise(discord, message)
		return
	}

	if message.MessageReference != nil && message.ReferencedMessage != nil && message.ReferencedMessage.Author.ID == config.GetConfig().App.BotID {
		convID, ok := h.conversationMap.GetConversationByRef(message.MessageReference.MessageID)
		if ok {
			fmt.Println("Found conversation ID:", convID)
			h.generateFollowUpChat(discord, message, client, ctx, intent, history)
			return
		}
	}

	fmt.Println("Could not find conversation for reference message")
	fmt.Println("Generating new chat...")
	h.generateNewChat(discord, message, client, ctx, intent, history)
}

func reactToNoise(discord *discordgo.Session, message *discordgo.MessageCreate) {
	reactions := []string{"❌", "🤫", "🙄", "📉"}
	selected := reactions[rand.Intn(len(reactions))]
	err := discord.MessageReactionAdd(message.ChannelID, message.ID, selected)
	if err != nil {
		fmt.Println("Error adding reaction:", err)
	}
}
