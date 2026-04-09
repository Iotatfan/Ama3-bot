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

	targetUID := "none"
	targetRole := "external"
	senderRole := "external"
	combinedContent := ""
	replyTarget := message.Reference()
	refMsg := message.ReferencedMessage

	if message.Author.ID == config.GetConfig().App.OwnerID {
		senderRole = "doctor"
	}

	if refMsg != nil && refMsg.Author.ID == config.GetConfig().App.OwnerID {
		targetRole = "doctor"
		replyTarget = refMsg.Reference()
	}

	if refMsg != nil && refMsg.Author.ID != config.GetConfig().App.BotID {
		targetUID = refMsg.Author.ID
		combinedContent = fmt.Sprintf("[INTENT:%s][UID:%s][SENDER_ROLE:%s][TARGET_UID:%s][TARGET_CONTEXT:%s][TARGET_ROLE:%s] %s.", intent, message.Author.ID, senderRole, targetUID, refMsg.Content, targetRole, message.Content)
	} else {
		combinedContent = fmt.Sprintf("[INTENT:%s][UID:%s][SENDER_ROLE:%s] %s", intent, message.Author.ID, senderRole, message.Content)
	}

	if history != "" {
		combinedContent = fmt.Sprintf("%s\n[CONVERSATION HISTORY]\n%s", combinedContent, history)
	}

	userContent := []responses.ResponseInputContentUnionParam{
		{
			OfInputText: &responses.ResponseInputTextParam{
				Text: combinedContent,
			},
		},
	}

	if message.Attachments != nil || (message.ReferencedMessage != nil && message.ReferencedMessage.Attachments != nil) {
		attachments := message.Attachments
		if message.ReferencedMessage != nil {
			attachments = append(attachments, message.ReferencedMessage.Attachments...)
		}

		for _, att := range attachments {
			if strings.HasPrefix(att.ContentType, "image/") {
				userContent = append(userContent, responses.ResponseInputContentUnionParam{
					OfInputImage: &responses.ResponseInputImageParam{
						ImageURL: openai.String(att.URL),
					},
				})
			}
		}
	}

	input := responses.ResponseNewParamsInputUnion{
		OfInputItemList: []responses.ResponseInputItemUnionParam{
			{
				OfMessage: &responses.EasyInputMessageParam{
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(config.GetConfig().AI.Prompts.System),
					},
					Role: responses.EasyInputMessageRoleSystem,
				},
			},
			{
				OfMessage: &responses.EasyInputMessageParam{
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(strings.Replace(config.GetConfig().AI.Prompts.IdentityRule, "{{.OwnerID}}", config.GetConfig().App.OwnerID, -1)),
					},
					Role: responses.EasyInputMessageRoleSystem,
				},
			},
			{
				OfMessage: &responses.EasyInputMessageParam{
					Role: responses.EasyInputMessageRoleDeveloper,
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(config.GetConfig().AI.Prompts.Developer),
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
		// Tools: []responses.ToolUnionParam{{
		// 	OfFunction: &responses.FunctionToolParam{
		// 		Name:        "web_search",
		// 		Description: openai.String("Search web for up to date info"),
		// 		Parameters: map[string]any{
		// 			"type": "object",
		// 			"properties": map[string]any{
		// 				"query": map[string]any{
		// 					"type":        "string",
		// 					"description": "Search query",
		// 				},
		// 			},
		// 			"required": []string{"query"},
		// 		},
		// 	},
		// }},
	})

	// for _, item := range resp.Output {
	// 	fmt.Println("GPT Resp", item.Type)
	// 	if item.Type == "function_call" {
	// 		toolCall := item.AsFunctionCall()
	// 		if toolCall.Name == "web_search" {
	// 			var args WebSearchInput
	// 			json.Unmarshal([]byte(toolCall.Arguments), &args)
	// 			fmt.Println("Query", args.Query)

	// 			resp2, err := client.Responses.New(ctx, responses.ResponseNewParams{
	// 				Model:              openai.ChatModelGPT5Mini,
	// 				PreviousResponseID: openai.String(resp.ID),
	// 				// Instructions:       openai.String(viper.GetString("GPT_SYSTEM_PROMPT")),
	// 				Input: responses.ResponseNewParamsInputUnion{
	// 					OfInputItemList: []responses.ResponseInputItemUnionParam{{
	// 						OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
	// 							CallID: toolCall.CallID,
	// 							Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
	// 								OfString: openai.String(args.Query),
	// 							},
	// 						},
	// 					}},
	// 				},
	// 			})

	// 			fmt.Println("GPT Resp: ", resp2)

	// 			return resp2, err
	// 		}
	// 	}
	// }
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
