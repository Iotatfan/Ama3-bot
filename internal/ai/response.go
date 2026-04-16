package ai

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/conversations"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

func (h *AIHandler) generateNewChat(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, intent Intent, history string, userSummary string) {
	stopTyping := h.typingManager.Start(discord, message.ChannelID)
	defer stopTyping()

	if intent == IntentNoise {
		reactToNoise(discord, message)
		return
	}

	conv, err := client.Conversations.New(ctx, conversations.ConversationNewParams{})
	if err != nil {
		fmt.Println("error generating response:", err)
		return
	}

	resp, replyTarget, err := generateAIResponse(message, client, ctx, conv.ID, intent, history, userSummary)
	if err != nil {
		fmt.Println("error generating response:", err)
		return
	}

	h.sendReplyMessage(discord, message, resp.OutputText(), replyTarget, conv.ID)
}

func (h *AIHandler) generateFollowUpChat(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, intent Intent, history string, userSummary string) {
	stopTyping := h.typingManager.Start(discord, message.ChannelID)
	defer stopTyping()

	if intent == IntentNoise {
		reactToNoise(discord, message)
		return
	}

	refID := message.MessageReference.MessageID
	convID, ok := h.conversationMap.GetConversationByRef(refID)
	if !ok {
		return
	}
	fmt.Println("Generating follow-up chat for conversation ID:", convID)

	resp, replyTarget, err := generateAIResponse(message, client, ctx, convID, intent, history, userSummary)
	if err != nil {
		fmt.Println(err)
		return
	}

	h.sendReplyMessage(discord, message, resp.OutputText(), replyTarget, convID)
}

func generateAIResponse(message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, convID string, intent Intent, history string, userSummary string) (*responses.Response, *discordgo.MessageReference, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	cfg := config.GetConfig()
	combinedContent, replyTarget := buildCombinedUserContent(cfg, message, intent, history, userSummary)
	userContent := buildUserContent(combinedContent, message)
	input := buildResponseInput(cfg, userContent)

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input: input,
		Model: openai.ChatModelGPT5_4,
		// Conversation: responses.ResponseNewParamsConversationUnion{
		// 	OfConversationObject: &responses.ResponseConversationParam{
		// 		ID: convID,
		// 	},
		// },
		Reasoning: shared.ReasoningParam{
			Effort: conversations.ReasoningEffortMedium,
		},
		PromptCacheRetention: responses.ResponseNewParamsPromptCacheRetention24h,
	})

	if err == nil {
		return resp, replyTarget, nil
	}

	if strings.Contains(err.Error(), "quota") || strings.Contains(err.Error(), "rate") || strings.Contains(err.Error(), "limit") {
		fmt.Println("Fallback to lighter model")

		fallbackResp, fallbackErr := client.Responses.New(ctx, responses.ResponseNewParams{
			Input: input,
			Model: openai.ChatModelGPT5_4Mini,
			// Conversation: responses.ResponseNewParamsConversationUnion{
			// 	OfConversationObject: &responses.ResponseConversationParam{
			// 		ID: convID,
			// 	},
			// },
			Reasoning: shared.ReasoningParam{
				Effort: conversations.ReasoningEffortMedium,
			},
		})
		if fallbackErr != nil {
			fmt.Println("Fallback also failed:", fallbackErr)
			return nil, nil, fallbackErr
		}
		return fallbackResp, replyTarget, nil
	}

	return nil, nil, err
}

func buildCombinedUserContent(cfg *config.Config, message *discordgo.MessageCreate, intent Intent, history string, userSummary string) (string, *discordgo.MessageReference) {
	targetUID := "none"
	targetRole := "external"
	senderRole := "external"
	replyTarget := message.Reference()
	refMsg := message.ReferencedMessage
	refMsgContent := ""

	ownerID := ""
	botID := ""
	if cfg != nil {
		ownerID = cfg.App.OwnerID
		botID = cfg.App.BotID
	}

	if message.Author != nil && message.Author.ID == ownerID {
		senderRole = "doctor"
	}

	if refMsg != nil && refMsg.Author != nil && refMsg.Author.ID == ownerID {
		targetRole = "doctor"
		replyTarget = refMsg.Reference()
		refMsgContent = refMsg.Content
	}

	if refMsg != nil && len(refMsg.Embeds) > 0 {
		embedContents := make([]string, 0, len(refMsg.Embeds))
		for _, embed := range refMsg.Embeds {
			if embed.Title != "" {
				embedContents = append(embedContents, embed.Title)
			}
			if embed.Description != "" {
				embedContents = append(embedContents, embed.Description)
			}
		}
		if len(embedContents) > 0 {
			refMsgContent += "\n" + strings.Join(embedContents, "\n")
		}
	}
	var combinedContent string

	if userSummary != "" {
		combinedContent = fmt.Sprintf("%s\n[SUBJECT_SUMMARY]\n%s", combinedContent, userSummary)
	}
	if history != "" {
		combinedContent = fmt.Sprintf("%s\n[CONVERSATION HISTORY]\n%s", combinedContent, history)
	}

	if refMsg != nil && refMsg.Author != nil && refMsg.Author.ID != botID {
		targetUID = refMsg.Author.ID
		combinedContent = fmt.Sprintf("[INTENT:%s]\n[UID:%s]\n[SENDER_ROLE:%s]\n[TARGET_UID:%s]\n[TARGET_CONTEXT:%s]\n[TARGET_ROLE:%s]\n[LATEST_MESSAGE:%s].", intent, message.Author.ID, senderRole, targetUID, refMsgContent, targetRole, message.Content)
	} else {
		combinedContent = fmt.Sprintf("[INTENT:%s]\n[UID:%s]\n[SENDER_ROLE:%s]\n[LATEST_MESSAGE:%s]", intent, message.Author.ID, senderRole, message.Content)
	}

	return combinedContent, replyTarget
}

func buildUserContent(combinedContent string, message *discordgo.MessageCreate) []responses.ResponseInputContentUnionParam {
	userContent := []responses.ResponseInputContentUnionParam{
		{
			OfInputText: &responses.ResponseInputTextParam{
				Text: combinedContent,
			},
		},
	}

	for _, imageURL := range collectAttachments(message) {
		userContent = append(userContent, responses.ResponseInputContentUnionParam{
			OfInputImage: &responses.ResponseInputImageParam{
				ImageURL: openai.String(imageURL),
			},
		})

	}

	return userContent
}

func collectAttachments(message *discordgo.MessageCreate) []string {
	if message == nil {
		return nil
	}

	imageUrls := make([]string, 0, len(message.Attachments))
	for _, att := range message.Attachments {
		if strings.HasPrefix(att.ContentType, "image/") {
			imageUrls = append(imageUrls, att.URL)
		}
	}

	for _, embed := range message.Embeds {
		if embed.Image != nil && embed.Image.URL != "" {
			imageUrls = append(imageUrls, embed.Image.URL)
		}
	}

	if message.ReferencedMessage != nil {
		for _, att := range message.ReferencedMessage.Attachments {
			if strings.HasPrefix(att.ContentType, "image/") {
				imageUrls = append(imageUrls, att.URL)
			}
		}

		for _, embed := range message.ReferencedMessage.Embeds {
			if embed.Image != nil && embed.Image.URL != "" {
				imageUrls = append(imageUrls, embed.Image.URL)
			}
		}
	}

	return imageUrls
}

func buildResponseInput(cfg *config.Config, userContent []responses.ResponseInputContentUnionParam) responses.ResponseNewParamsInputUnion {
	systemPrompt := ""
	identityPrompt := ""
	developerPrompt := ""

	if cfg != nil {
		systemPrompt = strings.ReplaceAll(cfg.AI.Prompts.System, "{{.OwnerID}}", cfg.App.OwnerID)
		identityPrompt = strings.ReplaceAll(cfg.AI.Prompts.IdentityRule, "{{.OwnerID}}", cfg.App.OwnerID)
		developerPrompt = strings.ReplaceAll(cfg.AI.Prompts.Developer, "{{.OwnerID}}", cfg.App.OwnerID)

	}

	return responses.ResponseNewParamsInputUnion{
		OfInputItemList: []responses.ResponseInputItemUnionParam{
			{
				OfMessage: &responses.EasyInputMessageParam{
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(systemPrompt),
					},
					Role: responses.EasyInputMessageRoleSystem,
				},
			},
			{
				OfMessage: &responses.EasyInputMessageParam{
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(identityPrompt),
					},
					Role: responses.EasyInputMessageRoleSystem,
				},
			},
			{
				OfMessage: &responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRoleDeveloper,
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(developerPrompt),
					},
				},
			},
			{
				OfMessage: &responses.EasyInputMessageParam{
					Content: responses.EasyInputMessageContentUnionParam{
						OfInputItemContentList: userContent,
					},
					Role: responses.EasyInputMessageRoleUser,
				},
			},
		},
	}
}

func (h *AIHandler) sendReplyMessage(discord *discordgo.Session, message *discordgo.MessageCreate, content string, replyTarget *discordgo.MessageReference, convID string) {
	// Discord has a message character limit of 2000, so split long responses.
	if len(content) > 2000 {
		chunks := smartSentenceChunk(content, 2000)
		msgRef := message.Reference()

		for _, chunk := range chunks {
			sent, err := discord.ChannelMessageSendReply(message.ChannelID, chunk, msgRef)
			if err != nil {
				fmt.Println(err)
				return
			}

			msgRef = sent.Reference()
			h.conversationMap.Set(convID, sent.ID)
			time.Sleep(300 * time.Millisecond)
		}
		return
	}

	sent, err := discord.ChannelMessageSendReply(message.ChannelID, content, replyTarget)
	if err != nil {
		fmt.Println(err)
		if err.Error() == `HTTP 403 Forbidden, {"message": "Missing Permissions", "code": 50013}` {
			fmt.Println("Insufficient permissions to send message")
			dmChannel, err := discord.UserChannelCreate(message.Author.ID)
			if err != nil {
				fmt.Println("Failed to create DM channel:", err)
				return
			}

			_, err = discord.ChannelMessageSend(dmChannel.ID, "I don't have permission to reply in that channel.")
			if err != nil {
				fmt.Println("Failed to send DM message:", err)
			}
		}
		return
	}
	h.conversationMap.Set(convID, sent.ID)
}

func smartSentenceChunk(text string, limit int) []string {
	var chunks []string

	for len(text) > limit {
		cut := -1

		// Look backwards for sentence or paragraph end.
		for i := limit; i > limit-400 && i > 0; i-- {
			c := text[i]
			if c == '.' || c == '!' || c == '?' || c == '\n' {
				cut = i + 1
				break
			}
		}

		if cut == -1 {
			cut = limit // fallback if no sentence end found
		}

		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}

	if len(text) > 0 {
		chunks = append(chunks, text)
	}

	return chunks
}

func (h *AIHandler) GenerateUserSummary(username string, userSummary string, messages []string, client *openai.Client, ctx context.Context) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	summaryPrompt := config.GetConfig().AI.Prompts.Summary
	summaryPrompt = strings.Replace(summaryPrompt, "{{.OldSummary}}", userSummary, 1)
	summaryPrompt = strings.Replace(summaryPrompt, "{{.NewMessages}}", strings.Join(messages, "\n"), 1)
	summaryPrompt = strings.Replace(summaryPrompt, "{{.Username}}", username, 1)

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(summaryPrompt),
		},
		Model: openai.ChatModelGPT5_4Mini,
	})
	if err != nil {
		fmt.Println("error determining intent:", err)
		return "", err
	}

	return resp.OutputText(), nil
}

func reactToNoise(discord *discordgo.Session, message *discordgo.MessageCreate) {
	reactions := []string{"❌", "🤫", "🙄", "📉"}
	selected := reactions[rand.Intn(len(reactions))]
	err := discord.MessageReactionAdd(message.ChannelID, message.ID, selected)
	if err != nil {
		fmt.Println("Error adding reaction:", err)
	}
}
