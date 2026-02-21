package chatGpt

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/conversations"
	"github.com/openai/openai-go/v3/responses"
	"github.com/spf13/viper"
)

// In-memory mapping of conversation IDs to Discord message IDs.
// This is used to keep track of which GPT conversation corresponds to which Discord message
var conversationMap = NewConversationMap()

type ConversationMap struct {
	convToRef map[string]string
	refToConv map[string]string
}

func NewConversationMap() *ConversationMap {
	return &ConversationMap{
		convToRef: make(map[string]string),
		refToConv: make(map[string]string),
	}
}

func (m *ConversationMap) Set(convID, refID string) {
	m.convToRef[convID] = refID
	m.refToConv[refID] = convID
}

func (m *ConversationMap) GetRef(convID string) (string, bool) {
	ref, ok := m.convToRef[convID]
	return ref, ok
}

func (m *ConversationMap) GetConversationByRef(refID string) (string, bool) {
	conv, ok := m.refToConv[refID]
	return conv, ok
}

func (m *ConversationMap) Delete(convID string) {
	if ref, ok := m.convToRef[convID]; ok {
		delete(m.convToRef, convID)
		delete(m.refToConv, ref)
	}
}

func isBotMentioned(message *discordgo.MessageCreate) bool {
	botID := viper.GetString("BOT_ID")
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

	return msg.Author.ID == viper.GetString("BOT_ID")
}

func ParseGptMessage(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	if message.Author.ID == viper.GetString("BOT_ID") || message.Author.Bot {
		fmt.Println("SKIP")
		return
	}

	if !isBotMentioned(message) && !isReplyToBot(discord, message) {
		return
	}

	fmt.Println("Ref:", message.MessageReference)
	if message.MessageReference != nil && message.ReferencedMessage.Author.ID == viper.GetString("BOT_ID") {
		convID, ok := conversationMap.GetConversationByRef(message.MessageReference.MessageID)
		if ok {
			fmt.Println("Found conversation ID:", convID)
			generateFollowUpChat(discord, message, client, ctx)
			return
		}
	}

	fmt.Println("Could not find conversation for reference message")
	fmt.Println("Generating new chat...")
	generateNewChat(discord, message, client, ctx)
	return
}

func generateNewChat(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	conv, err := client.Conversations.New(ctx, conversations.ConversationNewParams{})
	if err != nil {
		panic(err)
	}

	resp, err := generateGptResponse(message, client, ctx, conv.ID)

	if err != nil {
		fmt.Println("error generating response:", err)
		return
	}

	sendReplyMessage(discord, message, resp.OutputText(), conv.ID)
}

func generateFollowUpChat(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	refID := message.MessageReference.MessageID
	convID, ok := conversationMap.GetConversationByRef(refID)
	if !ok {
		// fallback: go to generate chat flow
		return
	}
	fmt.Println("Generating follow-up chat for conversation ID:", convID)

	resp, err := generateGptResponse(message, client, ctx, convID)

	if err != nil {
		fmt.Println(err)
		return
	}

	sendReplyMessage(discord, message, resp.OutputText(), convID)
}

func generateGptResponse(message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, convID string) (*responses.Response, error) {
	input := responses.ResponseNewParamsInputUnion{
		// OfString: openai.String(message.Content),
		OfInputItemList: []responses.ResponseInputItemUnionParam{{
			OfMessage: &responses.EasyInputMessageParam{
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: openai.String(message.Content)},
				Role: responses.EasyInputMessageRoleUser,
			},
		}},
	}

	if message.Attachments != nil || (message.ReferencedMessage != nil && message.ReferencedMessage.Attachments != nil) {
		attachments := message.Attachments
		if message.ReferencedMessage != nil {
			attachments = append(attachments, message.ReferencedMessage.Attachments...)
		}

		for _, att := range attachments {
			if strings.HasPrefix(att.ContentType, "image/") {
				fmt.Println("Adding attachment URL to input:", att.URL)
				input.OfInputItemList = append(input.OfInputItemList, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Content: responses.EasyInputMessageContentUnionParam{
							OfInputItemContentList: []responses.ResponseInputContentUnionParam{
								{
									OfInputImage: &responses.ResponseInputImageParam{
										ImageURL: openai.String(att.URL),
									},
								},
							},
						},
						Role: responses.EasyInputMessageRoleUser,
					},
				})
			}
		}
	}

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input:        input,
		Model:        openai.ChatModelGPT5Mini,
		Instructions: openai.String(viper.GetString("GPT_SYSTEM_PROMPT")),
		Conversation: responses.ResponseNewParamsConversationUnion{
			OfConversationObject: &responses.ResponseConversationParam{
				ID: convID,
			},
		},
	})

	return resp, err
}

func sendReplyMessage(discord *discordgo.Session, message *discordgo.MessageCreate, content string, convID string) {
	// Discord has a message character limit of 2000, so we need to split the response into multiple messages if it's too long
	if len(content) > 2000 {
		respText := content
		msgRef := message.Reference()
		for len(respText) > 0 {
			chunk := respText
			if len(chunk) > 1990 {
				chunk = respText[:1990]
			}
			sent, err := discord.ChannelMessageSendReply(message.ChannelID, chunk, msgRef)
			if err != nil {
				fmt.Println(err)
				return
			}
			msgRef = sent.Reference()
			conversationMap.Set(convID, sent.ID)
			respText = respText[len(chunk):]
		}
	} else {
		sent, err := discord.ChannelMessageSendReply(message.ChannelID, content, message.Reference())
		if err != nil {
			fmt.Println(err)
			return
		}
		conversationMap.Set(convID, sent.ID)
	}
}
