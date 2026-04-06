package ai

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/conversations"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// In-memory mapping of conversation IDs to Discord message IDs.
// This is used to keep track of which AI conversation corresponds to which Discord message
var conversationMap = NewConversationMap()
var typingManager = NewTypingManager()

type ConversationMap struct {
	mu        sync.RWMutex
	convToRef map[string]string
	refToConv map[string]string
}

type Intent string

const (
	IntentDirect            Intent = "direct"
	IntentReplyToTarget     Intent = "reply_to_target"
	IntentAskAbout          Intent = "ask_about_target"
	IntentValidationRequest Intent = "validation_request"
	IntentActionOnSelf      Intent = "action_on_self"
	IntentInterjection      Intent = "interjection"
	Unknown                 Intent = "unknown"
)

type typingWorker struct {
	refCount int
	stopCh   chan struct{}
}

type TypingManager struct {
	mu      sync.Mutex
	workers map[string]*typingWorker
}

func NewConversationMap() *ConversationMap {
	return &ConversationMap{
		convToRef: make(map[string]string),
		refToConv: make(map[string]string),
	}
}

func NewTypingManager() *TypingManager {
	return &TypingManager{
		workers: make(map[string]*typingWorker),
	}
}

func (m *ConversationMap) Set(convID, refID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if oldRef, ok := m.convToRef[convID]; ok && oldRef != refID {
		delete(m.refToConv, oldRef)
	}

	m.convToRef[convID] = refID
	m.refToConv[refID] = convID
}

func (m *ConversationMap) GetRef(convID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ref, ok := m.convToRef[convID]
	return ref, ok
}

func (m *ConversationMap) GetConversationByRef(refID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conv, ok := m.refToConv[refID]
	return conv, ok
}

func (m *ConversationMap) Delete(convID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ref, ok := m.convToRef[convID]; ok {
		delete(m.convToRef, convID)
		delete(m.refToConv, ref)
	}
}

func (m *TypingManager) Start(discord *discordgo.Session, channelID string) func() {
	m.mu.Lock()
	if worker, ok := m.workers[channelID]; ok {
		worker.refCount++
		m.mu.Unlock()
		return m.stopFn(channelID)
	}

	worker := &typingWorker{
		refCount: 1,
		stopCh:   make(chan struct{}),
	}
	m.workers[channelID] = worker
	m.mu.Unlock()

	go m.run(discord, channelID, worker.stopCh)
	return m.stopFn(channelID)
}

func (m *TypingManager) stopFn(channelID string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			m.Stop(channelID)
		})
	}
}

func (m *TypingManager) Stop(channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	worker, ok := m.workers[channelID]
	if !ok {
		return
	}

	worker.refCount--
	if worker.refCount > 0 {
		return
	}

	close(worker.stopCh)
	delete(m.workers, channelID)
}

func (m *TypingManager) run(discord *discordgo.Session, channelID string, stopCh <-chan struct{}) {
	_ = discord.ChannelTyping(channelID)

	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			_ = discord.ChannelTyping(channelID)
		}
	}
}

type WebSearchInput struct {
	Query string `json:"query"`
}

func isBotMentioned(message *discordgo.MessageCreate) bool {
	botID := config.GetConfig().App.BotID
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

	return msg.Author.ID == config.GetConfig().App.BotID
}

func stripBotMention(content string) string {
	botID := config.GetConfig().App.BotID
	replacer := strings.NewReplacer(
		"<@"+botID+">", "",
		"<@!"+botID+">", "",
	)

	return strings.TrimSpace(strings.Join(strings.Fields(replacer.Replace(content)), " "))
}

func determineIntent(message *discordgo.MessageCreate, ctx context.Context, client *openai.Client, isReplyFlow bool) Intent {
	intentPrompt := ""
	if isReplyFlow {
		intentPrompt = strings.Replace(config.GetConfig().AI.Prompts.IntentReply, "{{.Message}}", message.Content, 1)
		fmt.Println("Determining intent with prompt:", intentPrompt)
	} else {
		intentPrompt = strings.Replace(config.GetConfig().AI.Prompts.Intent, "{{.Message}}", message.Content, 1)
	}
	fmt.Println("Determining intent with prompt:", intentPrompt)

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(intentPrompt),
		},
		Model: openai.ChatModelGPT5_4Mini,
	})
	if err != nil {
		fmt.Println("error determining intent:", err)
		return Unknown
	}

	fmt.Println("Intent response:", resp.OutputText())
	// Parse the intent from the AI response
	switch resp.OutputText() {
	case "direct":
		return IntentDirect
	case "reply_to_target":
		return IntentReplyToTarget
	case "ask_about_target":
		return IntentAskAbout
	case "validation_request":
		return IntentValidationRequest
	default:
		return Unknown
	}
}

func calculateInterestScore(message *discordgo.MessageCreate, ctx context.Context, client *openai.Client, discord *discordgo.Session) (float32, string) {
	// Get past message for context, combine with current message, and feed into interest scoring prompt
	combinedContent := ""
	pastMessages, err := discord.ChannelMessages(message.ChannelID, config.GetConfig().AI.Interest.PastMessageLimit, "", "", "")
	if err != nil {
		fmt.Println("error fetching past messages for interest scoring:", err)
		combinedContent = message.Content
	} else {
		for i := len(pastMessages) - 1; i >= 0; i-- {
			m := pastMessages[i]
			if m.ID == message.ID {
				continue
			}
			combinedContent += fmt.Sprintf("[UID:%s] %s : %s\n", m.Author.ID, m.Author.Username, m.Content)
		}
	}
	interjectionPrompt := strings.Replace(config.GetConfig().AI.Prompts.InterestScore, "{{.Message}}", message.Content, 1)
	interjectionPrompt = strings.Replace(interjectionPrompt, "{{.History}}", combinedContent, 1)
	interjectionPrompt = strings.Replace(interjectionPrompt, "{{.OwnerID}}", config.GetConfig().App.OwnerID, 1)

	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(interjectionPrompt),
		},
		Model: openai.ChatModelGPT5_4Nano,
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

func handlePotentialInterjection(message *discordgo.MessageCreate, ctx context.Context, client *openai.Client, discord *discordgo.Session) (bool, string) {
	score, interjectionMsg := calculateInterestScore(message, ctx, client, discord)

	if score > float32(config.GetConfig().AI.Interest.InterestScoreThreshold) {
		return true, interjectionMsg
	}

	return false, ""
}

var channelCooldownMap = struct {
	mu         sync.RWMutex
	lastActive map[string]time.Time
}{
	lastActive: make(map[string]time.Time),
}

func isNotCooldown(channelId string) bool {
	// Check if any conversation has been active in the channel in the last cooldown period
	cooldown := time.Duration(config.GetConfig().AI.Interest.CooldownSeconds) * time.Second
	now := time.Now()

	channelCooldownMap.mu.RLock()
	defer channelCooldownMap.mu.RUnlock()

	lastActive, ok := channelCooldownMap.lastActive[channelId]
	if !ok {
		return true
	}

	return now.Sub(lastActive) > cooldown
}

func updateChannelActivity(channelID string) {
	channelCooldownMap.mu.Lock()
	defer channelCooldownMap.mu.Unlock()

	channelCooldownMap.lastActive[channelID] = time.Now()
}

func ParseMessage(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context) {
	if message.Author.ID == config.GetConfig().App.BotID || message.Author.Bot {
		fmt.Println("SKIP")
		return
	}

	if !isBotMentioned(message) && !isReplyToBot(discord, message) && config.GetConfig().AI.Interest.EnableInterestDetection {
		if !isNotCooldown(message.ChannelID) {
			fmt.Println("Channel is in cooldown, skipping interest check")
			return
		}

		shouldHandle, history := handlePotentialInterjection(message, ctx, client, discord)
		if shouldHandle {
			updateChannelActivity(message.ChannelID)

			fmt.Println("Message is not directed at bot and has high interest score, generating interjection response...")
			message.Content = stripBotMention(message.Content)
			intent := determineIntent(message, ctx, client, false)

			generateNewChat(discord, message, client, ctx, intent, history)
			return
		}
		fmt.Println("Message is not directed at bot and has low interest score, skipping...")
		return
	}

	message.Content = stripBotMention(message.Content)
	intent := determineIntent(message, ctx, client, message.ReferencedMessage != nil)

	fmt.Println("Ref:", message.MessageReference)
	if message.MessageReference != nil && message.ReferencedMessage != nil && message.ReferencedMessage.Author.ID == config.GetConfig().App.BotID {
		convID, ok := conversationMap.GetConversationByRef(message.MessageReference.MessageID)
		if ok {
			fmt.Println("Found conversation ID:", convID)
			generateFollowUpChat(discord, message, client, ctx, intent)
			return
		}
	}

	fmt.Println("Could not find conversation for reference message")
	fmt.Println("Generating new chat...")
	generateNewChat(discord, message, client, ctx, intent, "")
}

func generateNewChat(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, intent Intent, history string) {
	stopTyping := typingManager.Start(discord, message.ChannelID)
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

	sendReplyMessage(discord, message, resp.OutputText(), replyTarget, conv.ID)
}

func generateFollowUpChat(discord *discordgo.Session, message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, intent Intent) {
	stopTyping := typingManager.Start(discord, message.ChannelID)
	defer stopTyping()

	refID := message.MessageReference.MessageID
	convID, ok := conversationMap.GetConversationByRef(refID)
	if !ok {
		// fallback: go to generate chat flow
		return
	}
	fmt.Println("Generating follow-up chat for conversation ID:", convID)

	resp, replyTarget, err := generateAIResponse(message, client, ctx, convID, intent, "")

	if err != nil {
		fmt.Println(err)
		return
	}

	sendReplyMessage(discord, message, resp.OutputText(), replyTarget, convID)
}

func generateAIResponse(message *discordgo.MessageCreate, client *openai.Client, ctx context.Context, convID string, intent Intent, history string) (*responses.Response, *discordgo.MessageReference, error) {
	targetUID := "none"
	role := "external"
	combinedContent := ""
	replyTarget := message.Reference()
	refMsg := message.ReferencedMessage

	isReplyToExternal := intent == IntentReplyToTarget &&
		refMsg != nil &&
		refMsg.Author.ID != config.GetConfig().App.OwnerID
	isOwnerMessage := message.Author.ID == config.GetConfig().App.OwnerID

	if isOwnerMessage {
		role = "doctor"
	}

	if isReplyToExternal {
		role = "doctor"
		replyTarget = refMsg.Reference()
	}

	if refMsg != nil && refMsg.Author.ID != config.GetConfig().App.BotID {
		targetUID = refMsg.Author.ID
		combinedContent = fmt.Sprintf("[INTENT:%s][UID:%s][ROLE:%s][TARGET_UID:%s][TARGET_CONTEXT:%s] %s.", intent, message.Author.ID, role, targetUID, refMsg.Content, message.Content)
	} else {
		combinedContent = fmt.Sprintf("[INTENT:%s][UID:%s][ROLE:%s] %s", intent, message.Author.ID, role, message.Content)
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

	fmt.Println(userContent[0].OfInputText.Text)

	if message.Attachments != nil || (message.ReferencedMessage != nil && message.ReferencedMessage.Attachments != nil) {
		attachments := message.Attachments
		if message.ReferencedMessage != nil {
			attachments = append(attachments, message.ReferencedMessage.Attachments...)
		}

		for _, att := range attachments {
			if strings.HasPrefix(att.ContentType, "image/") {
				fmt.Println("Adding attachment URL to input:", att.URL)
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
						OfString: openai.String(config.GetConfig().AI.Prompts.System)},
					Role: responses.EasyInputMessageRoleSystem,
				},
			},
			{
				OfMessage: &responses.EasyInputMessageParam{
					Content: responses.EasyInputMessageContentUnionParam{
						OfString: openai.String(strings.Replace(config.GetConfig().AI.Prompts.IdentityRule, "{{.OwnerID}}", config.GetConfig().App.OwnerID, -1))},
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
		return resp, replyTarget, err
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

func sendReplyMessage(discord *discordgo.Session, message *discordgo.MessageCreate, content string, replyTarget *discordgo.MessageReference, convID string) {
	// Discord has a message character limit of 2000, so we need to split the response into multiple messages if it's too long
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
			conversationMap.Set(convID, sent.ID)

			time.Sleep(300 * time.Millisecond)
		}
	} else {
		sent, err := discord.ChannelMessageSendReply(message.ChannelID, content, replyTarget)
		if err != nil {
			fmt.Println(err)
			return
		}
		conversationMap.Set(convID, sent.ID)
	}
}

func smartSentenceChunk(text string, limit int) []string {
	var chunks []string

	for len(text) > limit {
		cut := -1

		// Look backwards for sentence or paragraph end
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
