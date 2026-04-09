package ai

import (
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/config"
)

// In-memory mapping of conversation IDs to Discord message IDs.
// This is used to keep track of which AI conversation corresponds to which Discord message.
var conversationMap = NewConversationMap()
var typingManager = NewTypingManager()

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

var channelCooldownMap = struct {
	mu         sync.RWMutex
	lastActive map[string]time.Time
}{
	lastActive: make(map[string]time.Time),
}

var directFlowLimiter = struct {
	mu          sync.Mutex
	userLastReq map[string]time.Time
	chanLastReq map[string]time.Time
}{
	userLastReq: make(map[string]time.Time),
	chanLastReq: make(map[string]time.Time),
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

func isNotCooldown(channelID string) bool {
	// Check if any conversation has been active in the channel in the last cooldown period.
	cooldown := time.Duration(config.GetConfig().AI.Interest.CooldownSeconds) * time.Second
	now := time.Now()

	channelCooldownMap.mu.RLock()
	defer channelCooldownMap.mu.RUnlock()

	lastActive, ok := channelCooldownMap.lastActive[channelID]
	if !ok {
		return true
	}

	return now.Sub(lastActive) > cooldown
}

func updateChannelActivity(channelID string) {
	channelCooldownMap.mu.Lock()
	defer channelCooldownMap.mu.Unlock()

	if len(channelCooldownMap.lastActive) > 1000 {
		channelCooldownMap.lastActive = make(map[string]time.Time)
	}

	channelCooldownMap.lastActive[channelID] = time.Now()
}

func allowDirectFlow(userID, channelID string) bool {
	if !config.GetConfig().AI.Runtime.EnableDirectThrottle {
		return true
	}

	now := time.Now()

	directFlowLimiter.mu.Lock()
	defer directFlowLimiter.mu.Unlock()

	if len(directFlowLimiter.userLastReq) > maxDirectLimiterEntries() {
		directFlowLimiter.userLastReq = make(map[string]time.Time)
	}
	if len(directFlowLimiter.chanLastReq) > maxDirectLimiterEntries() {
		directFlowLimiter.chanLastReq = make(map[string]time.Time)
	}

	if last, ok := directFlowLimiter.userLastReq[userID]; ok && now.Sub(last) < directFlowUserCooldown() {
		return false
	}
	if last, ok := directFlowLimiter.chanLastReq[channelID]; ok && now.Sub(last) < directFlowChanCooldown() {
		return false
	}

	directFlowLimiter.userLastReq[userID] = now
	directFlowLimiter.chanLastReq[channelID] = now
	return true
}
