package repository

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/iotatfan/sora-go/internal/config"
	"github.com/iotatfan/sora-go/internal/crypto"
	"gorm.io/gorm"
)

type Memory struct {
	ID, Content, Category              string
	Confidence, Importance, Similarity float64
	Embedding                          []float64
}
type SelfMemory struct {
	ID, Content, Category              string
	Confidence, Importance, Similarity float64
	SourceMessageID                    string
	Embedding                          []float64
}
type MemoryConflict struct{ ID, SelfMemoryID, ProposedContent, SourceMessageID, Status string }
type MemoryRepository interface {
	CreateMemory(uid, content, category string, confidence, importance float64, embedding []float64, sourceMessage, guild, channel string) error
	SearchMemories(uid string, embedding []float64, limit int, minSimilarity, minConfidence float64) ([]Memory, error)
	SearchMemoryCandidates(uid string, embedding []float64, limit int, minConfidence float64) ([]Memory, error)
	SearchMemoriesByKeyword(uid, query string, limit int, minConfidence float64) ([]Memory, error)
	HasMemoryContent(uid, content string) (bool, error)
	CreateSelfMemory(content, category string, confidence, importance float64, embedding []float64, sourceMessage, guild, channel string) error
	SearchSelfMemories(embedding []float64, limit int, minSimilarity float64) ([]SelfMemory, error)
	SearchSelfMemoriesByKeyword(query string, limit int) ([]SelfMemory, error)
	HasSelfMemoryContent(content string) (bool, error)
	ListSelfMemories() ([]SelfMemory, error)
	ListConflicts() ([]MemoryConflict, error)
	GetConflict(id string) (MemoryConflict, error)
	ResolveConflict(id, action, ownerID string, embedding []float64) error
	DeleteSelfMemory(id string) error
}
type memoryRepository struct {
	db      *gorm.DB
	encrypt bool
}

func normalizeMemoryContent(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)), " ")
}

var memoryTokenRE = regexp.MustCompile(`[[:alnum:]]+`)

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

func parseVector(s string) []float64 {
	s = strings.TrimSpace(strings.Trim(s, "[]"))
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
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
	var rows []struct{ Content string }
	if err := r.db.Raw(`SELECT content FROM user_memories WHERE discord_uid=? AND active=true`, uid).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		existing, err := r.reveal(row.Content)
		if err == nil && normalizeMemoryContent(existing) == normalizeMemoryContent(content) {
			return nil
		}
	}
	return r.db.Exec(`INSERT INTO user_memories (id,discord_uid,content,category,confidence,importance,embedding,source_message_id,source_guild_id,source_channel_id,active,created_at,updated_at) VALUES (gen_random_uuid(),?,?,?,?,?,?::vector,?,?,?,true,?,?)`, uid, c, cat, confidence, importance, vectorLiteral(embedding), sourceMessage, guild, channel, time.Now(), time.Now()).Error
}

func (r *memoryRepository) HasMemoryContent(uid, content string) (bool, error) {
	if r.db == nil {
		return false, nil
	}
	var rows []struct{ Content string }
	if err := r.db.Raw(`SELECT content FROM user_memories WHERE discord_uid=? AND active=true`, uid).Scan(&rows).Error; err != nil {
		return false, err
	}
	for _, row := range rows {
		existing, err := r.reveal(row.Content)
		if err == nil && normalizeMemoryContent(existing) == normalizeMemoryContent(content) {
			return true, nil
		}
	}
	return false, nil
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
		ID, Content, Category, Embedding   string
		Confidence, Importance, Similarity float64
	}
	err := r.db.Raw(`SELECT id,content,category,embedding::text AS embedding,confidence,importance,1-(embedding <=> ?::vector) AS similarity FROM user_memories WHERE discord_uid=? AND active=true AND confidence>=? ORDER BY embedding <=> ?::vector LIMIT ?`, vectorLiteral(embedding), uid, minConfidence, vectorLiteral(embedding), limit).Scan(&rows).Error
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
		out = append(out, Memory{ID: row.ID, Content: c, Category: cat, Confidence: row.Confidence, Importance: row.Importance, Similarity: row.Similarity, Embedding: parseVector(row.Embedding)})
	}
	return out, nil
}

func keywordOverlap(query, content string) float64 {
	q := map[string]bool{}
	for _, token := range memoryTokenRE.FindAllString(strings.ToLower(query), -1) {
		if !map[string]bool{"a": true, "an": true, "the": true, "is": true, "am": true, "are": true, "my": true, "me": true, "i": true, "to": true, "of": true, "and": true, "or": true, "in": true, "on": true, "what": true, "do": true, "you": true}[token] {
			q[token] = true
		}
	}
	if len(q) == 0 {
		return 0
	}
	c := map[string]bool{}
	for _, token := range memoryTokenRE.FindAllString(strings.ToLower(content), -1) {
		c[token] = true
	}
	hits := 0
	for token := range q {
		if c[token] {
			hits++
		}
	}
	return float64(hits) / float64(len(q))
}

func (r *memoryRepository) SearchMemoriesByKeyword(uid, query string, limit int, minConfidence float64) ([]Memory, error) {
	if r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}
	var rows []struct {
		ID, Content, Category  string
		Confidence, Importance float64
	}
	if err := r.db.Raw(`SELECT id,content,category,confidence,importance FROM user_memories WHERE discord_uid=? AND active=true AND confidence>=?`, uid, minConfidence).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Memory, 0, len(rows))
	for _, row := range rows {
		content, err := r.reveal(row.Content)
		if err != nil {
			continue
		}
		category, err := r.reveal(row.Category)
		if err != nil {
			continue
		}
		score := keywordOverlap(query, content)
		if score > 0 {
			out = append(out, Memory{ID: row.ID, Content: content, Category: category, Confidence: row.Confidence, Importance: row.Importance, Similarity: score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Similarity == out[j].Similarity {
			return out[i].Importance > out[j].Importance
		}
		return out[i].Similarity > out[j].Similarity
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *memoryRepository) CreateSelfMemory(content, category string, confidence, importance float64, embedding []float64, sourceMessage, guild, channel string) error {
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
	var rows []struct{ Content string }
	if err := r.db.Raw(`SELECT content FROM self_memories WHERE active=true`).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		existing, err := r.reveal(row.Content)
		if err == nil && normalizeMemoryContent(existing) == normalizeMemoryContent(content) {
			return nil
		}
	}
	return r.db.Exec(`INSERT INTO self_memories (id,content,category,confidence,importance,embedding,source_message_id,source_guild_id,source_channel_id,active,created_at,updated_at) VALUES (gen_random_uuid(),?,?,?,?,?::vector,?,?,?,true,?,?)`, c, cat, confidence, importance, vectorLiteral(embedding), sourceMessage, guild, channel, time.Now(), time.Now()).Error
}

func (r *memoryRepository) HasSelfMemoryContent(content string) (bool, error) {
	if r.db == nil {
		return false, nil
	}
	var rows []struct{ Content string }
	if err := r.db.Raw(`SELECT content FROM self_memories WHERE active=true`).Scan(&rows).Error; err != nil {
		return false, err
	}
	for _, row := range rows {
		existing, err := r.reveal(row.Content)
		if err == nil && normalizeMemoryContent(existing) == normalizeMemoryContent(content) {
			return true, nil
		}
	}
	return false, nil
}
func (r *memoryRepository) SearchSelfMemories(embedding []float64, limit int, minSimilarity float64) ([]SelfMemory, error) {
	if r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	var rows []struct {
		ID, Content, Category, Embedding string
		Confidence, Importance           float64
	}
	if minSimilarity <= 0 {
		minSimilarity = 0.78
	}
	err := r.db.Raw(`SELECT id,content,category,embedding::text AS embedding,confidence,importance FROM self_memories WHERE active=true AND embedding IS NOT NULL AND 1-(embedding <=> ?::vector) >= ? ORDER BY embedding <=> ?::vector LIMIT ?`, vectorLiteral(embedding), minSimilarity, vectorLiteral(embedding), limit).Scan(&rows).Error
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
		out = append(out, SelfMemory{ID: x.ID, Content: c, Category: cat, Confidence: x.Confidence, Importance: x.Importance, Embedding: parseVector(x.Embedding)})
	}
	return out, nil
}

func (r *memoryRepository) SearchSelfMemoriesByKeyword(query string, limit int) ([]SelfMemory, error) {
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
	if err := r.db.Raw(`SELECT id,content,category,confidence,importance FROM self_memories WHERE active=true`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SelfMemory, 0, len(rows))
	for _, row := range rows {
		content, err := r.reveal(row.Content)
		if err != nil {
			continue
		}
		category, err := r.reveal(row.Category)
		if err != nil {
			continue
		}
		if score := keywordOverlap(query, content); score > 0 {
			out = append(out, SelfMemory{ID: row.ID, Content: content, Category: category, Confidence: row.Confidence, Importance: row.Importance, Similarity: score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Similarity == out[j].Similarity {
			return out[i].Importance > out[j].Importance
		}
		return out[i].Similarity > out[j].Similarity
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (r *memoryRepository) ListSelfMemories() ([]SelfMemory, error) {
	if r.db == nil {
		return nil, nil
	}
	var rows []struct {
		ID, Content, Category  string
		Confidence, Importance float64
	}
	if err := r.db.Raw(`SELECT id,content,category,confidence,importance FROM self_memories WHERE active=true ORDER BY importance DESC, confidence DESC LIMIT 100`).Scan(&rows).Error; err != nil {
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
func (r *memoryRepository) ListConflicts() ([]MemoryConflict, error) {
	if r.db == nil {
		return nil, nil
	}
	var out []MemoryConflict
	err := r.db.Raw(`SELECT id,self_memory_id,proposed_content,source_message_id,status FROM self_memory_conflicts WHERE status='unresolved' ORDER BY created_at`).Scan(&out).Error
	return out, err
}
func (r *memoryRepository) GetConflict(id string) (MemoryConflict, error) {
	if r.db == nil {
		return MemoryConflict{}, nil
	}
	var out MemoryConflict
	err := r.db.Where("id = ? AND status = 'unresolved'", id).First(&out).Error
	return out, err
}
func (r *memoryRepository) ResolveConflict(id, action, ownerID string, embedding []float64) error {
	if r.db == nil {
		return nil
	}
	tx := r.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
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
		var err error
		if len(embedding) == 0 {
			err = tx.Exec(`UPDATE self_memories SET content=?,embedding=NULL,updated_at=now() WHERE id=?`, content, c.SelfMemoryID).Error
		} else {
			err = tx.Exec(`UPDATE self_memories SET content=?,embedding=?::vector,updated_at=now() WHERE id=?`, content, vectorLiteral(embedding), c.SelfMemoryID).Error
		}
		if err != nil {
			return err
		}
	}
	if err := tx.Exec(`UPDATE self_memory_conflicts SET status=?,resolved_by=?,resolved_at=now(),updated_at=now() WHERE id=?`, status, ownerID, id).Error; err != nil {
		return err
	}
	return tx.Commit().Error
}
func (r *memoryRepository) DeleteSelfMemory(id string) error {
	if r.db == nil {
		return nil
	}
	return r.db.Exec(`UPDATE self_memories SET active=false,updated_at=now() WHERE id=?`, id).Error
}
