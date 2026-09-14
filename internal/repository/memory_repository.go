package repository

import (
	"fmt"
	"strings"
	"time"

	"github.com/iotatfan/sora-go/internal/config"
	"github.com/iotatfan/sora-go/internal/crypto"
	"gorm.io/gorm"
)

type Memory struct {
	ID, Content, Category              string
	Confidence, Importance, Similarity float64
}
type SelfMemory struct {
	ID, Content, Category  string
	Confidence, Importance float64
	SourceMessageID        string
}
type MemoryConflict struct{ ID, SelfMemoryID, ProposedContent, SourceMessageID, Status string }
type MemoryRepository interface {
	CreateMemory(uid, content, category string, confidence, importance float64, embedding []float64, sourceMessage, guild, channel string) error
	SearchMemories(uid string, embedding []float64, limit int, minSimilarity, minConfidence float64) ([]Memory, error)
	SearchMemoryCandidates(uid string, embedding []float64, limit int, minConfidence float64) ([]Memory, error)
	CreateSelfMemory(content, category string, confidence, importance float64, sourceMessage, guild, channel string) error
	SearchSelfMemories(embedding []float64, limit int) ([]SelfMemory, error)
	ListSelfMemories() ([]SelfMemory, error)
	ListConflicts() ([]MemoryConflict, error)
	ResolveConflict(id, action, ownerID string) error
	DeleteSelfMemory(id string) error
}
type memoryRepository struct {
	db      *gorm.DB
	encrypt bool
}

func NewMemoryRepository(db *gorm.DB, encrypt bool) MemoryRepository {
	return &memoryRepository{db: db, encrypt: encrypt}
}

func vectorLiteral(v []float64) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = fmt.Sprintf("%.8f", n)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
func (r *memoryRepository) protect(s string) (string, error) {
	if !r.encrypt {
		return s, nil
	}
	k, e := config.GetEncryptionKey()
	if e != nil {
		return "", e
	}
	return crypto.Encrypt(k, s)
}
func (r *memoryRepository) reveal(s string) (string, error) {
	if !r.encrypt {
		return s, nil
	}
	k, e := config.GetEncryptionKey()
	if e != nil {
		return "", e
	}
	return crypto.Decrypt(k, s)
}

func (r *memoryRepository) CreateMemory(uid, content, category string, confidence, importance float64, embedding []float64, sourceMessage, guild, channel string) error {
	if r.db == nil {
		return nil
	}
	c, e := r.protect(content)
	if e != nil {
		return e
	}
	cat, e := r.protect(category)
	if e != nil {
		return e
	}
	return r.db.Exec(`INSERT INTO user_memories (id,discord_uid,content,category,confidence,importance,embedding,source_message_id,source_guild_id,source_channel_id,active,created_at,updated_at) VALUES (gen_random_uuid(),?,?,?,?,?,?::vector,?,?,?,true,?,?)`, uid, c, cat, confidence, importance, vectorLiteral(embedding), sourceMessage, guild, channel, time.Now(), time.Now()).Error
}

func (r *memoryRepository) SearchMemories(uid string, embedding []float64, limit int, minSimilarity, minConfidence float64) ([]Memory, error) {
	return r.SearchMemoryCandidates(uid, embedding, limit, minConfidence)
}

func (r *memoryRepository) SearchMemoryCandidates(uid string, embedding []float64, limit int, minConfidence float64) ([]Memory, error) {
	if r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	var rows []struct {
		ID, Content, Category              string
		Confidence, Importance, Similarity float64
	}
	err := r.db.Raw(`SELECT id,content,category,confidence,importance,1-(embedding <=> ?::vector) AS similarity FROM user_memories WHERE discord_uid=? AND active=true AND confidence>=? ORDER BY embedding <=> ?::vector LIMIT ?`, vectorLiteral(embedding), uid, minConfidence, vectorLiteral(embedding), limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]Memory, 0, len(rows))
	for _, row := range rows {
		c, e := r.reveal(row.Content)
		if e != nil {
			continue
		}
		cat, e := r.reveal(row.Category)
		if e != nil {
			continue
		}
		out = append(out, Memory{row.ID, c, cat, row.Confidence, row.Importance, row.Similarity})
	}
	return out, nil
}

func (r *memoryRepository) CreateSelfMemory(content, category string, confidence, importance float64, sourceMessage, guild, channel string) error {
	if r.db == nil {
		return nil
	}
	c, err := r.protect(content)
	if err != nil {
		return err
	}
	cat, err := r.protect(category)
	if err != nil {
		return err
	}
	return r.db.Exec(`INSERT INTO self_memories (id,content,category,confidence,importance,source_message_id,source_guild_id,source_channel_id,active,created_at,updated_at) VALUES (gen_random_uuid(),?,?,?,?,?,?,?,true,?,?)`, c, cat, confidence, importance, sourceMessage, guild, channel, time.Now(), time.Now()).Error
}
func (r *memoryRepository) SearchSelfMemories(embedding []float64, limit int) ([]SelfMemory, error) {
	if r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	var rows []struct {
		ID, Content, Category  string
		Confidence, Importance float64
	}
	err := r.db.Raw(`SELECT id,content,category,confidence,importance FROM self_memories WHERE active=true ORDER BY importance DESC, confidence DESC LIMIT ?`, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]SelfMemory, 0, len(rows))
	for _, x := range rows {
		c, e := r.reveal(x.Content)
		if e != nil {
			continue
		}
		cat, e := r.reveal(x.Category)
		if e != nil {
			continue
		}
		out = append(out, SelfMemory{ID: x.ID, Content: c, Category: cat, Confidence: x.Confidence, Importance: x.Importance})
	}
	return out, nil
}
func (r *memoryRepository) ListSelfMemories() ([]SelfMemory, error) {
	return r.SearchSelfMemories(nil, 100)
}
func (r *memoryRepository) ListConflicts() ([]MemoryConflict, error) {
	if r.db == nil {
		return nil, nil
	}
	var out []MemoryConflict
	err := r.db.Raw(`SELECT id,self_memory_id,proposed_content,source_message_id,status FROM self_memory_conflicts WHERE status='unresolved' ORDER BY created_at`).Scan(&out).Error
	return out, err
}
func (r *memoryRepository) ResolveConflict(id, action, ownerID string) error {
	if r.db == nil {
		return nil
	}
	tx := r.db.Begin()
	var c struct{ SelfMemoryID, ProposedContent string }
	if err := tx.Raw(`SELECT self_memory_id,proposed_content FROM self_memory_conflicts WHERE id=? AND status='unresolved'`, id).Scan(&c).Error; err != nil {
		return err
	}
	status := map[string]string{"keep_existing": "rejected", "accept_new": "accepted", "keep_both": "accepted", "dismiss": "dismissed"}[action]
	if status == "" {
		return fmt.Errorf("invalid conflict action")
	}
	if action == "accept_new" {
		content, e := r.protect(c.ProposedContent)
		if e != nil {
			return e
		}
		if err := tx.Exec(`UPDATE self_memories SET content=?,updated_at=now() WHERE id=?`, content, c.SelfMemoryID).Error; err != nil {
			return err
		}
	}
	return tx.Exec(`UPDATE self_memory_conflicts SET status=?,resolved_by=?,resolved_at=now(),updated_at=now() WHERE id=?`, status, ownerID, id).Error
}
func (r *memoryRepository) DeleteSelfMemory(id string) error {
	if r.db == nil {
		return nil
	}
	return r.db.Exec(`UPDATE self_memories SET active=false,updated_at=now() WHERE id=?`, id).Error
}
