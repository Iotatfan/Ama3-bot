package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/conversations"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

func (h *AIHandler) generateNewChat(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, intent Intent, history string) {
	stopTyping := h.typingManager.Start(discord, message.ChannelID)
	defer stopTyping()

	conv, err := client.Conversations.New(ctx, conversations.ConversationNewParams{})
	if err != nil {
		fmt.Println("error generating response:", err)
		return
	}

	resp, replyTarget, err := generateAIResponse(message, client, ctx, conv.ID, intent, history)
	if err != nil {
		fmt.Println("error generating response:", err)
		return
	}

	h.sendReplyMessage(discord, message, resp.OutputText(), replyTarget, conv.ID)
}

func (h *AIHandler) generateFollowUpChat(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, intent Intent, history string) {
	stopTyping := h.typingManager.Start(discord, message.ChannelID)
	defer stopTyping()

	refID := message.MessageReference.MessageID
	convID, ok := h.conversationMap.GetConversationByRef(refID)
	if !ok {
		// fallback: go to generate new chat flow
		return
	}
	fmt.Println("Generating follow-up chat for conversation ID:", convID)

	resp, replyTarget, err := generateAIResponse(message, client, ctx, convID, intent, history)
	if err != nil {
		fmt.Println(err)
		return
	}

	h.sendReplyMessage(discord, message, resp.OutputText(), replyTarget, convID)
}

func generateAIResponse(message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, convID string, intent Intent, history string) (*responses.Response, *discordgo.MessageReference, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	cfg := config.GetConfig()
	combinedContent, replyTarget := buildCombinedUserContent(cfg, message, intent, history)
	userContent := buildUserContent(combinedContent, message)
	input := buildResponseInput(cfg, userContent)

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input: input,
		Model: openai.ChatModelGPT5_4,
		Conversation: responses.ResponseNewParamsConversationUnion{
			OfConversationObject: &responses.ResponseConversationParam{
				ID: convID,
			},
		},
		Reasoning: shared.ReasoningParam{
			Effort: conversations.ReasoningEffortMedium,
		},
	})

	if err == nil {
		return resp, replyTarget, nil
	}

	if strings.Contains(err.Error(), "quota") || strings.Contains(err.Error(), "rate") || strings.Contains(err.Error(), "limit") {
		fmt.Println("Fallback to lighter model")

		fallbackResp, fallbackErr := client.Responses.New(ctx, responses.ResponseNewParams{
			Input: input,
			Model: openai.ChatModelGPT5_4Mini,
			Conversation: responses.ResponseNewParamsConversationUnion{
				OfConversationObject: &responses.ResponseConversationParam{
					ID: convID,
				},
			},
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

func buildCombinedUserContent(cfg *config.Config, message *discordgo.MessageCreate, intent Intent, history string) (string, *discordgo.MessageReference) {
	targetUID := "none"
	targetRole := "external"
	senderRole := "external"
	replyTarget := message.Reference()
	refMsg := message.ReferencedMessage

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
	}

	var combinedContent string
	if refMsg != nil && refMsg.Author != nil && refMsg.Author.ID != botID {
		targetUID = refMsg.Author.ID
		combinedContent = fmt.Sprintf("[INTENT:%s][UID:%s][SENDER_ROLE:%s][TARGET_UID:%s][TARGET_CONTEXT:%s][TARGET_ROLE:%s] %s.", intent, message.Author.ID, senderRole, targetUID, refMsg.Content, targetRole, message.Content)
	} else {
		combinedContent = fmt.Sprintf("[INTENT:%s][UID:%s][SENDER_ROLE:%s] %s", intent, message.Author.ID, senderRole, message.Content)
	}

	if history != "" {
		combinedContent = fmt.Sprintf("%s\n[CONVERSATION HISTORY]\n%s", combinedContent, history)
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

	for _, att := range collectAttachments(message) {
		if strings.HasPrefix(att.ContentType, "image/") {
			userContent = append(userContent, responses.ResponseInputContentUnionParam{
				OfInputImage: &responses.ResponseInputImageParam{
					ImageURL: openai.String(att.URL),
				},
			})
		}
	}

	return userContent
}

func collectAttachments(message *discordgo.MessageCreate) []*discordgo.MessageAttachment {
	if message == nil {
		return nil
	}

	attachments := make([]*discordgo.MessageAttachment, 0, len(message.Attachments))
	attachments = append(attachments, message.Attachments...)

	if message.ReferencedMessage != nil {
		attachments = append(attachments, message.ReferencedMessage.Attachments...)
	}

	return attachments
}

func buildResponseInput(cfg *config.Config, userContent []responses.ResponseInputContentUnionParam) responses.ResponseNewParamsInputUnion {
	systemPrompt := ""
	identityPrompt := ""
	developerPrompt := ""

	if cfg != nil {
		systemPrompt = cfg.AI.Prompts.System
		identityPrompt = strings.Replace(cfg.AI.Prompts.IdentityRule, "{{.OwnerID}}", cfg.App.OwnerID, -1)
		developerPrompt = cfg.AI.Prompts.Developer
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
