package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type HistoryMessage struct {
	Order   int    `json:"order"`
	UID     string `json:"uid"`
	Content string `json:"content"`

	label string
}

type HistorySummary struct {
	Order   int    `json:"order"`
	UID     string `json:"uid"`
	Summary string `json:"summary"`
}

type HistorySummaryResponse struct {
	Summaries []HistorySummary `json:"summaries"`
}

type historySummaryRequest struct {
	Format   string           `json:"format"`
	Messages []HistoryMessage `json:"messages"`
}

func buildHistoryMessages(pastMessages []*discordgo.Message, currentMessageID, botID string) []HistoryMessage {
	messages := make([]HistoryMessage, 0, len(pastMessages))
	order := 0
	for i := len(pastMessages) - 1; i >= 0; i-- {
		m := pastMessages[i]
		if m == nil || m.ID == currentMessageID {
			continue
		}

		content := historyMessageContent(m)
		if content == "" {
			continue
		}

		uid, label := historyAuthorLabel(m.Author, botID)
		messages = append(messages, HistoryMessage{Order: order, UID: uid, Content: content, label: label})
		order++
	}

	return messages
}

func renderHistoryMessages(messages []HistoryMessage) string {
	var builder strings.Builder
	for _, message := range messages {
		if message.label != "" {
			fmt.Fprintf(&builder, "[UID:%s] %s : %s\n", message.UID, message.label, message.Content)
		} else {
			fmt.Fprintf(&builder, "[UID:%s] : %s\n", message.UID, message.Content)
		}
	}
	return builder.String()
}

func (h *AIHandler) summarizeHistoryMessages(ctx context.Context, messages []HistoryMessage) string {
	original := renderHistoryMessages(messages)
	cfg := h.config()
	if cfg == nil || !cfg.AI.HistorySummary.Enabled {
		return original
	}

	threshold := cfg.AI.HistorySummary.MessageThresholdCharacters
	if threshold <= 0 {
		threshold = 500
	}
	longMessages := historyMessagesToSummarize(messages, threshold)
	if len(longMessages) == 0 {
		return original
	}

	requestJSON, err := json.Marshal(historySummaryRequest{Format: "json", Messages: longMessages})
	if err != nil {
		fmt.Printf("history_summary fallback reason=request_encode_failed long_messages=%d error=%v\n", len(longMessages), err)
		return original
	}

	maxSummaryCharacters := cfg.AI.HistorySummary.MaxSummaryCharacters
	if maxSummaryCharacters <= 0 {
		maxSummaryCharacters = 300
	}

	if h.client == nil {
		fmt.Printf("history_summary fallback reason=client_unavailable long_messages=%d\n", len(longMessages))
		return original
	}

	var response *responses.Response
	err = h.runModelCall(ctx, "history_summary", func() error {
		var callErr error
		response, callErr = h.client.Responses.New(ctx, responses.ResponseNewParams{
			Input:           responses.ResponseNewParamsInputUnion{OfString: openai.String(string(requestJSON))},
			Instructions:    openai.String(historySummaryInstructions(maxSummaryCharacters)),
			Model:           openai.ChatModelGPT5_4Mini,
			MaxOutputTokens: openai.Int(2000),
			Text: responses.ResponseTextConfigParam{
				Format: responses.ResponseFormatTextConfigUnionParam{
					OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
				},
			},
		})
		return callErr
	})
	if err != nil || response == nil {
		if err != nil {
			fmt.Printf("history_summary fallback reason=model_request_failed long_messages=%d error=%v\n", len(longMessages), err)
		} else {
			fmt.Printf("history_summary fallback reason=empty_model_response long_messages=%d\n", len(longMessages))
		}
		return original
	}
	logResponseUsage("history_summary", response)

	var parsed HistorySummaryResponse
	if err := json.Unmarshal([]byte(response.OutputText()), &parsed); err != nil {
		fmt.Printf("history_summary fallback reason=invalid_json long_messages=%d error=%v\n", len(longMessages), err)
		return original
	}
	if !validateHistorySummaries(longMessages, parsed.Summaries) {
		fmt.Printf("history_summary fallback reason=invalid_items expected=%d received=%d\n", len(longMessages), len(parsed.Summaries))
		return original
	}

	summaries := make(map[int]string, len(parsed.Summaries))
	for _, summary := range parsed.Summaries {
		summaries[summary.Order] = truncateHistorySummary(summary.Summary, maxSummaryCharacters)
	}

	result := make([]HistoryMessage, len(messages))
	copy(result, messages)
	for i := range result {
		if summary, ok := summaries[result[i].Order]; ok {
			result[i].Content = summary
		}
	}
	return renderHistoryMessages(result)
}

func historySummaryInstructions(maxSummaryCharacters int) string {
	return fmt.Sprintf("Summarize each message independently. Return only valid json in the form {\"summaries\":[{\"order\":0,\"uid\":\"...\",\"summary\":\"...\"}]}. Preserve every order and UID exactly. Each summary must be concise and no more than %d characters.", maxSummaryCharacters)
}

func historyMessagesToSummarize(messages []HistoryMessage, threshold int) []HistoryMessage {
	longMessages := make([]HistoryMessage, 0)
	for _, message := range messages {
		if len([]rune(message.Content)) > threshold {
			longMessages = append(longMessages, HistoryMessage{Order: message.Order, UID: message.UID, Content: message.Content})
		}
	}
	return longMessages
}

func validateHistorySummaries(messages []HistoryMessage, summaries []HistorySummary) bool {
	if len(messages) != len(summaries) {
		return false
	}
	expected := make(map[int]string, len(messages))
	for _, message := range messages {
		expected[message.Order] = message.UID
	}
	for _, summary := range summaries {
		uid, ok := expected[summary.Order]
		if !ok || uid != summary.UID || strings.TrimSpace(summary.Summary) == "" {
			return false
		}
		delete(expected, summary.Order)
	}
	return len(expected) == 0
}

func truncateHistorySummary(summary string, maxCharacters int) string {
	cleaned := strings.TrimSpace(summary)
	runes := []rune(cleaned)
	if len(runes) <= maxCharacters {
		return cleaned
	}
	return strings.TrimSpace(string(runes[:maxCharacters]))
}
