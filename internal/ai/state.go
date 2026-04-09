package ai

import (
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
)

var defaultAIHandler = NewAIHandler()

type AIHandler struct {
	conversationMap *ConversationMap
	typingManager   *TypingManager
	channelCooldown channelCooldownTracker
	directLimiter   directFlowLimiter
}

type ConversationMap struct {
	mu        sync.RWMutex
	convToRef map[string]conversationRef
	refToConv map[string]string
}

type conversationRef struct {
	refID     string
	updatedAt time.Time
}

type typingWorker struct {
	refCount int
	stopCh   chan struct{}
}

type TypingManager struct {
	mu      sync.Mutex
	workers map[string]*typingWorker
}

type channelCooldownTracker struct {
	mu         sync.RWMutex
	lastActive map[string]time.Time
}

type directFlowLimiter struct {
	mu          sync.Mutex
	userLastReq map[string]time.Time
	chanLastReq map[string]time.Time
}

func NewAIHandler() *AIHandler {
	return &AIHandler{
		conversationMap: NewConversationMap(),
		typingManager:   NewTypingManager(),
		channelCooldown: channelCooldownTracker{
			lastActive: make(map[string]time.Time),
		},
		directLimiter: directFlowLimiter{
			userLastReq: make(map[string]time.Time),
			chanLastReq: make(map[string]time.Time),
		},
	}
}

func NewConversationMap() *ConversationMap {
	return &ConversationMap{
		convToRef: make(map[string]conversationRef),
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

	m.pruneLocked()
	if len(m.convToRef) >= maxConversationMappings() {
		// Keep retention simple and predictable when map grows too large.
		m.convToRef = make(map[string]conversationRef)
		m.refToConv = make(map[string]string)
	}

	if oldRef, ok := m.convToRef[convID]; ok && oldRef.refID != refID {
		delete(m.refToConv, oldRef.refID)
	}

	m.convToRef[convID] = conversationRef{
		refID:     refID,
		updatedAt: time.Now(),
	}
	m.refToConv[refID] = convID
}

func (m *ConversationMap) GetRef(convID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ref, ok := m.convToRef[convID]
	return ref.refID, ok
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
		delete(m.refToConv, ref.refID)
	}
}

func (m *ConversationMap) pruneLocked() {
	if len(m.convToRef) == 0 {
		return
	}

	threshold := time.Now().Add(-conversationTTL())
	for convID, ref := range m.convToRef {
		if ref.updatedAt.Before(threshold) {
			delete(m.convToRef, convID)
			delete(m.refToConv, ref.refID)
		}
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

func conversationTTL() time.Duration {
	ttlSeconds := config.GetConfig().AI.Runtime.ConversationTTLSeconds
	if ttlSeconds <= 0 {
		return 6 * time.Hour
	}

	return time.Duration(ttlSeconds) * time.Second
}

func maxConversationMappings() int {
	limit := config.GetConfig().AI.Runtime.MaxConversationMappings
	if limit <= 0 {
		return 1000
	}

	return limit
}

func directFlowUserCooldown() time.Duration {
	seconds := config.GetConfig().AI.Runtime.DirectFlowUserCooldown
	if seconds <= 0 {
		return 3 * time.Second
	}

	return time.Duration(seconds) * time.Second
}

func directFlowChanCooldown() time.Duration {
	seconds := config.GetConfig().AI.Runtime.DirectFlowChanCooldown
	if seconds <= 0 {
		return time.Second
	}

	return time.Duration(seconds) * time.Second
}

func maxDirectLimiterEntries() int {
	limit := config.GetConfig().AI.Runtime.MaxDirectLimiterEntries
	if limit <= 0 {
		return 4000
	}

	return limit
}

func (h *AIHandler) isNotCooldown(channelID string) bool {
	// Check if any conversation has been active in the channel in the last cooldown period.
	cooldown := time.Duration(config.GetConfig().AI.Interest.CooldownSeconds) * time.Second
	now := time.Now()

	h.channelCooldown.mu.RLock()
	defer h.channelCooldown.mu.RUnlock()

	lastActive, ok := h.channelCooldown.lastActive[channelID]
	if !ok {
		return true
	}

	return now.Sub(lastActive) > cooldown
}

func (h *AIHandler) updateChannelActivity(channelID string) {
	h.channelCooldown.mu.Lock()
	defer h.channelCooldown.mu.Unlock()

	if len(h.channelCooldown.lastActive) > 1000 {
		h.channelCooldown.lastActive = make(map[string]time.Time)
	}

	h.channelCooldown.lastActive[channelID] = time.Now()
}

func (h *AIHandler) allowDirectFlow(userID, channelID string) bool {
	if !config.GetConfig().AI.Runtime.EnableDirectThrottle {
		return true
	}

	now := time.Now()

	h.directLimiter.mu.Lock()
	defer h.directLimiter.mu.Unlock()

	if len(h.directLimiter.userLastReq) > maxDirectLimiterEntries() {
		h.directLimiter.userLastReq = make(map[string]time.Time)
	}
	if len(h.directLimiter.chanLastReq) > maxDirectLimiterEntries() {
		h.directLimiter.chanLastReq = make(map[string]time.Time)
	}

	if last, ok := h.directLimiter.userLastReq[userID]; ok && now.Sub(last) < directFlowUserCooldown() {
		return false
	}
	if last, ok := h.directLimiter.chanLastReq[channelID]; ok && now.Sub(last) < directFlowChanCooldown() {
		return false
	}

	h.directLimiter.userLastReq[userID] = now
	h.directLimiter.chanLastReq[channelID] = now
	return true
}
