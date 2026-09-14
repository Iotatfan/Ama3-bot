package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/repository"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"regexp"
	"sort"
	"strings"
	"time"
)

type memoryGate struct {
	NeedsMemory bool   `json:"needs_memory"`
	Query       string `json:"query"`
}
type memoryCandidate struct {
	Content    string  `json:"content"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	Importance float64 `json:"importance"`
}
type memoryExtraction struct {
	Memories []memoryCandidate `json:"memories"`
}
type selfExtraction struct {
	Memories []memoryCandidate `json:"memories"`
}

func (h *AIHandler) extractSelfMemory(ctx context.Context, text, sourceID, guildID, channelID string) {
	c := h.config().AI.Memory
	if !c.Enabled || !c.SelfEnabled || h.memoryRepo == nil || strings.TrimSpace(text) == "" {
		return
	}
	p := `Return JSON only as {"memories":[{"content":"...","category":"decision|commitment|preference|identity|goal","confidence":0.0,"importance":0.0}]}. Extract only durable facts explicitly expressed by the assistant. Empty list for ordinary answers. ASSISTANT RESPONSE: ` + text
	model := c.SelfExtractionModel
	if model == "" {
		model = c.ExtractionModel
	}
	r, err := h.client.Responses.New(ctx, responses.ResponseNewParams{Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(p)}, Model: openai.ChatModel(model)})
	if err != nil {
		fmt.Printf("self-memory extraction failed: %v\n", err)
		return
	}
	var out selfExtraction
	if json.Unmarshal([]byte(strings.TrimSpace(r.OutputText())), &out) != nil {
		return
	}
	for _, x := range out.Memories {
		if strings.TrimSpace(x.Content) == "" || x.Confidence < c.MinConfidence {
			continue
		}
		v, e := h.embed(ctx, x.Content)
		if e != nil {
			continue
		}
		if e = h.memoryRepo.CreateSelfMemory(x.Content, x.Category, x.Confidence, x.Importance, v, sourceID, guildID, channelID); e != nil {
			fmt.Printf("self-memory write failed: %v\n", e)
			continue
		}
		_ = v
	}
}

var memoryTokenRE = regexp.MustCompile(`[[:alnum:]]+`)
var memoryStopWords = map[string]bool{"a": true, "an": true, "the": true, "is": true, "am": true, "are": true, "my": true, "me": true, "i": true, "to": true, "of": true, "and": true, "or": true, "in": true, "on": true, "what": true, "do": true, "you": true}

var memoryRetrievalPhrases = []string{"remember", "forgot", "saved", "previously", "last time", "we discussed", "you said"}
var memoryRetrievalWords = map[string]bool{"my": true, "mine": true, "i": true, "me": true, "prefer": true, "preference": true, "like": true, "goal": true, "project": true, "plan": true, "work": true}
var memoryNoise = map[string]bool{"hi": true, "hello": true, "hey": true, "bye": true, "goodbye": true, "ok": true, "okay": true, "thanks": true, "thank": true, "lol": true, "got": true, "it": true}

func shouldRetrieveMemory(query string, enabled bool, minTokens int) bool {
	if !enabled {
		return true
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	for _, phrase := range memoryRetrievalPhrases {
		if strings.Contains(q, phrase) {
			return true
		}
	}
	tokens := memoryTokenRE.FindAllString(q, -1)
	if len(tokens) == 0 {
		return false
	}
	if minTokens <= 0 {
		minTokens = 3
	}
	if len(tokens) < minTokens {
		return false
	}
	allNoise := true
	for _, token := range tokens {
		if !memoryNoise[token] {
			allNoise = false
			break
		}
	}
	if allNoise {
		return false
	}
	for _, token := range tokens {
		if memoryRetrievalWords[token] {
			return true
		}
	}
	return false
}

func memoryKeywordScore(query, content string) float64 {
	q := map[string]bool{}
	for _, t := range memoryTokenRE.FindAllString(strings.ToLower(query), -1) {
		if !memoryStopWords[t] {
			q[t] = true
		}
	}
	if len(q) == 0 {
		return 0
	}
	c := map[string]bool{}
	for _, t := range memoryTokenRE.FindAllString(strings.ToLower(content), -1) {
		c[t] = true
	}
	hits := 0
	for t := range q {
		if c[t] {
			hits++
		}
	}
	return float64(hits) / float64(len(q))
}

func (h *AIHandler) embed(ctx context.Context, text string) ([]float64, error) {
	c := h.config()
	model := c.AI.Memory.EmbeddingModel
	if model == "" {
		model = "text-embedding-3-small"
	}
	r, e := h.client.Embeddings.New(ctx, openai.EmbeddingNewParams{Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(text)}, Model: openai.EmbeddingModel(model)})
	if e != nil {
		return nil, e
	}
	if len(r.Data) == 0 {
		return nil, fmt.Errorf("empty embedding")
	}
	return r.Data[0].Embedding, nil
}

func (h *AIHandler) memoryGate(ctx context.Context, m *discordgo.MessageCreate, history, summary string) (memoryGate, error) {
	p := fmt.Sprintf("Return JSON only: {\"needs_memory\":true|false,\"query\":\"...\",\"reason\":\"...\"}. Decide whether durable personal memory is needed. Use false for greetings, generic questions, or messages answerable from current history. MESSAGE: %s\nSUMMARY: %s\nHISTORY: %s", m.Content, summary, history)
	r, e := h.client.Responses.New(ctx, responses.ResponseNewParams{Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(p)}, Model: openai.ChatModel(h.config().AI.Memory.RetrievalGateModel), Metadata: shared.Metadata{"discord_user_id": m.Author.ID}})
	if e != nil {
		return memoryGate{}, e
	}
	var g memoryGate
	e = json.Unmarshal([]byte(strings.TrimSpace(r.OutputText())), &g)
	return g, e
}

func (h *AIHandler) retrieveMemories(ctx context.Context, m *discordgo.MessageCreate, history, summary string) ([]repository.Memory, []repository.SelfMemory, error) {
	c := h.config().AI.Memory
	if !c.Enabled || c.RetrievalMode == "off" || h.memoryRepo == nil {
		if !c.Enabled {
		}
		if c.Enabled && c.RetrievalMode == "off" {
		}
		return nil, nil, nil
	}
	q := m.Content
	if c.RetrievalMode == "model" {
		g, e := h.memoryGate(ctx, m, history, summary)
		if e != nil || !g.NeedsMemory {
			if e != nil {
				fmt.Printf("memory gate failed user_id=%s error=%v\n", m.Author.ID, e)
			} else {
			}
			return nil, nil, e
		}
		if g.Query != "" {
			q = g.Query
		}
	}
	if !shouldRetrieveMemory(q, c.LocalGateEnabled, c.LocalGateMinQueryTokens) {
		return nil, nil, nil
	}
	v, e := h.embed(ctx, q)
	if e != nil {
		fmt.Printf("memory query embedding failed user_id=%s error=%v\n", m.Author.ID, e)
		return nil, nil, e
	}
	pool := c.CandidatePoolSize
	if pool <= 0 {
		pool = 50
	}
	memories, e := h.memoryRepo.SearchMemoryCandidates(m.Author.ID, v, pool, c.MinConfidence)
	if e != nil {
		fmt.Printf("memory search failed user_id=%s error=%v\n", m.Author.ID, e)
		return nil, nil, e
	}
	type scored struct {
		m repository.Memory
		s float64
	}
	scoredMem := []scored{}
	vw := c.VectorWeight
	if vw <= 0 {
		vw = .65
	}
	kw := c.KeywordWeight
	if kw <= 0 {
		kw = .35
	}
	for _, memory := range memories {
		ks := memoryKeywordScore(q, memory.Content)
		score := vw*memory.Similarity + kw*ks
		if score >= (func() float64 {
			if c.MinCombinedScore > 0 {
				return c.MinCombinedScore
			}
			return .35
		})() && (ks >= (func() float64 {
			if c.MinKeywordScore > 0 {
				return c.MinKeywordScore
			}
			return .20
		})() || memory.Similarity >= c.MinSimilarity) {
			scoredMem = append(scoredMem, scored{memory, score})
		}
	}
	sort.Slice(scoredMem, func(i, j int) bool { return scoredMem[i].s > scoredMem[j].s })
	max := c.MaxInjected
	if max <= 0 {
		max = 5
	}
	if len(scoredMem) > max {
		scoredMem = scoredMem[:max]
	}
	out := make([]repository.Memory, 0, len(scoredMem))
	for _, x := range scoredMem {
		out = append(out, x.m)
	}
	selfLimit := c.SelfMaxInjected
	if selfLimit <= 0 {
		selfLimit = 10
	}
	self, err := h.memoryRepo.SearchSelfMemories(v, selfLimit, c.SelfMinSimilarity)
	if err != nil {
		return out, nil, err
	}
	return out, self, nil
}

func formatMemories(mem []repository.Memory, max int) string {
	var b strings.Builder
	for _, m := range mem {
		line := "- " + m.Content
		if b.Len()+len(line)+1 > max {
			break
		}
		b.WriteString(line + "\n")
	}
	formatted := strings.TrimSpace(b.String())
	return formatted
}

func formatSelfMemories(mem []repository.SelfMemory, max int) string {
	var b strings.Builder
	for _, m := range mem {
		line := "- " + m.Content
		if b.Len()+len(line)+1 > max {
			break
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimSpace(b.String())
}

func (h *AIHandler) extractMemories(ctx context.Context, m *discordgo.MessageCreate, history, summary string) {
	c := h.config().AI.Memory
	if !c.Enabled || !c.WriteEnabled || h.memoryRepo == nil {
		if c.Enabled && !c.WriteEnabled {
			fmt.Printf("memory extraction skipped user_id=%s reason=write_disabled\n", m.Author.ID)
		}
		return
	}
	p := fmt.Sprintf("Return JSON only as {\"memories\":[{\"content\":\"...\",\"category\":\"preference|fact|goal|project|commitment|relationship\",\"confidence\":0.0,\"importance\":0.0}]}. Extract only durable useful personal facts explicitly supported by context. Empty list for casual or temporary content. MESSAGE: %s\nSUMMARY: %s\nRECENT CONTEXT: %s", m.Content, summary, history)
	r, e := h.client.Responses.New(ctx, responses.ResponseNewParams{Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(p)}, Model: openai.ChatModel(c.ExtractionModel), Metadata: shared.Metadata{"discord_user_id": m.Author.ID}})
	if e != nil {
		fmt.Printf("memory extraction failed user_id=%s error=%v\n", m.Author.ID, e)
		return
	}
	var out memoryExtraction
	if json.Unmarshal([]byte(strings.TrimSpace(r.OutputText())), &out) != nil {
		fmt.Printf("memory extraction invalid_json user_id=%s\n", m.Author.ID)
		return
	}
	accepted := 0
	for _, x := range out.Memories {
		if strings.TrimSpace(x.Content) == "" || x.Confidence < c.MinConfidence {
			continue
		}
		v, e := h.embed(ctx, x.Content)
		if e != nil {
			fmt.Printf("memory embedding failed user_id=%s category=%s error=%v\n", m.Author.ID, x.Category, e)
			continue
		}
		if e = h.memoryRepo.CreateMemory(m.Author.ID, x.Content, x.Category, x.Confidence, x.Importance, v, m.ID, m.GuildID, m.ChannelID); e != nil {
			fmt.Printf("memory write failed user_id=%s category=%s error=%v\n", m.Author.ID, x.Category, e)
			continue
		}
		accepted++
	}
}

func (h *AIHandler) queueMemory(m *discordgo.MessageCreate, summary string) {
	c := h.config().AI.Memory
	if h.memoryBuffer == nil || !c.Enabled || !c.WriteEnabled {
		return
	}
	now := time.Now()
	h.memoryBuffer.mu.Lock()
	items := h.memoryBuffer.items[m.Author.ID]
	items = append(items, PendingMemoryMessage{ID: m.ID, Content: m.Content, GuildID: m.GuildID, ChannelID: m.ChannelID, CreatedAt: now})
	h.memoryBuffer.items[m.Author.ID] = items
	if _, ok := h.memoryBuffer.first[m.Author.ID]; !ok {
		h.memoryBuffer.first[m.Author.ID] = now
	}
	size := c.ExtractionBatchSize
	if size <= 0 {
		size = 5
	}
	flush := len(items) >= size || now.Sub(h.memoryBuffer.first[m.Author.ID]) >= time.Duration(c.ExtractionFlushSeconds)*time.Second
	if flush {
		delete(h.memoryBuffer.items, m.Author.ID)
		delete(h.memoryBuffer.first, m.Author.ID)
	}
	h.memoryBuffer.mu.Unlock()
	if flush {
		history := ""
		for _, x := range items {
			history += x.Content + "\n"
		}
		go h.extractMemories(context.Background(), &discordgo.MessageCreate{Message: &discordgo.Message{Author: m.Author, ID: m.ID, Content: history, GuildID: m.GuildID, ChannelID: m.ChannelID}}, "", summary)
	}
}
