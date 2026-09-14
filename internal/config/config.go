package config

import (
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"github.com/iotatfan/sora-go/internal/helper"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	App      AppConfig      `mapstructure:"app" yaml:"app"`
	Auth     AuthConfig     `mapstructure:"auth" yaml:"auth"`
	Platform PlatformConfig `mapstructure:"platform" yaml:"platform"`
	AI       AIConfig       `mapstructure:"ai" yaml:"ai"`
	Database DatabaseConfig `mapstructure:"database" yaml:"database"`
	Security SecurityConfig `mapstructure:"security" yaml:"security"`
	Commands CommandsConfig `mapstructure:"commands" yaml:"commands"`
}

type SecurityConfig struct {
	// EncryptionKey is a 64-character hex string (32 bytes) used for AES-256-GCM
	// encryption of sensitive fields at rest. Leave empty to disable encryption.
	EncryptionKey string `mapstructure:"encryption_key" yaml:"encryption_key"`
}

type AppConfig struct {
	BotID   string `mapstructure:"bot_id" yaml:"bot_id"`
	OwnerID string `mapstructure:"owner_id" yaml:"owner_id"`
	RoleID  string `mapstructure:"role_id" yaml:"role_id"`
}

type AuthConfig struct {
	DiscordToken string `mapstructure:"discord_token" yaml:"discord_token"`
	OpenAIKey    string `mapstructure:"openai_key" yaml:"openai_key"`
}

type DatabaseConfig struct {
	DSN             string `mapstructure:"dsn" yaml:"dsn"`
	MaxOpenConns    int    `mapstructure:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime string `mapstructure:"conn_max_lifetime" yaml:"conn_max_lifetime"`
}

type PlatformConfig struct {
	WhitelistGuilds []string          `mapstructure:"whitelist_guilds" yaml:"whitelist_guilds"`
	Replacements    ReplacementConfig `mapstructure:"replacements" yaml:"replacements"`
}

type ReplacementConfig struct {
	Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
	Twitter   string `mapstructure:"twitter" yaml:"twitter"`
	Instagram string `mapstructure:"instagram" yaml:"instagram"`
}

type CommandsConfig struct {
	Enabled bool               `mapstructure:"enabled" yaml:"enabled"`
	Items   map[string]Command `mapstructure:"items" yaml:"items"`
}

type Command struct {
	Name        string `mapstructure:"name" yaml:"name"`
	Description string `mapstructure:"description" yaml:"description"`
	Content     string `mapstructure:"content" yaml:"content"`
}

type InterestConfig struct {
	InterestScoreThreshold  float64 `mapstructure:"interest_score_threshold" yaml:"interest_score_threshold"`
	PastMessageLimit        int     `mapstructure:"past_message_limit" yaml:"past_message_limit"`
	CooldownSeconds         int     `mapstructure:"cooldown_seconds" yaml:"cooldown_seconds"`
	EnableInterestDetection bool    `mapstructure:"enable_interest_detection" yaml:"enable_interest_detection"`
}

type RuntimeConfig struct {
	EnableDirectThrottle    bool `mapstructure:"enable_direct_throttle" yaml:"enable_direct_throttle"`
	ConversationTTLSeconds  int  `mapstructure:"conversation_ttl_seconds" yaml:"conversation_ttl_seconds"`
	MaxConversationMappings int  `mapstructure:"max_conversation_mappings" yaml:"max_conversation_mappings"`
	DirectFlowUserCooldown  int  `mapstructure:"direct_flow_user_cooldown_seconds" yaml:"direct_flow_user_cooldown_seconds"`
	DirectFlowChanCooldown  int  `mapstructure:"direct_flow_channel_cooldown_seconds" yaml:"direct_flow_channel_cooldown_seconds"`
	MaxDirectLimiterEntries int  `mapstructure:"max_direct_limiter_entries" yaml:"max_direct_limiter_entries"`
}

type AIConfig struct {
	Personality string         `mapstructure:"personality" yaml:"personality"`
	Prompts     PromptConfig   `mapstructure:"prompts" yaml:"prompts"`
	Interest    InterestConfig `mapstructure:"interest" yaml:"interest"`
	Runtime     RuntimeConfig  `mapstructure:"runtime" yaml:"runtime"`
	Summary     SummaryConfig  `mapstructure:"summary" yaml:"summary"`
	Memory      MemoryConfig   `mapstructure:"memory" yaml:"memory"`
}

type MemoryConfig struct {
	LocalGateEnabled             bool    `mapstructure:"local_gate_enabled" yaml:"local_gate_enabled"`
	LocalGateMinQueryTokens      int     `mapstructure:"local_gate_min_query_tokens" yaml:"local_gate_min_query_tokens"`
	SelfMinSimilarity            float64 `mapstructure:"self_min_similarity" yaml:"self_min_similarity"`
	SelfEnabled                  bool    `mapstructure:"self_enabled" yaml:"self_enabled"`
	SelfExtractionModel          string  `mapstructure:"self_extraction_model" yaml:"self_extraction_model"`
	SelfMaxInjected              int     `mapstructure:"self_max_injected" yaml:"self_max_injected"`
	SelfMaxInjectedCharacters    int     `mapstructure:"self_max_injected_characters" yaml:"self_max_injected_characters"`
	ConflictNotificationsEnabled bool    `mapstructure:"conflict_notifications_enabled" yaml:"conflict_notifications_enabled"`
	AdminChannelID               string  `mapstructure:"admin_channel_id" yaml:"admin_channel_id"`
	Enabled                      bool    `mapstructure:"enabled" yaml:"enabled"`
	WriteEnabled                 bool    `mapstructure:"write_enabled" yaml:"write_enabled"`
	RetrievalMode                string  `mapstructure:"retrieval_mode" yaml:"retrieval_mode"`
	ExtractionModel              string  `mapstructure:"extraction_model" yaml:"extraction_model"`
	RetrievalGateModel           string  `mapstructure:"retrieval_gate_model" yaml:"retrieval_gate_model"`
	EmbeddingModel               string  `mapstructure:"embedding_model" yaml:"embedding_model"`
	EmbeddingDimensions          int     `mapstructure:"embedding_dimensions" yaml:"embedding_dimensions"`
	MaxCandidates                int     `mapstructure:"max_candidates" yaml:"max_candidates"`
	MaxInjected                  int     `mapstructure:"max_injected" yaml:"max_injected"`
	MaxInjectedCharacters        int     `mapstructure:"max_injected_characters" yaml:"max_injected_characters"`
	MinSimilarity                float64 `mapstructure:"min_similarity" yaml:"min_similarity"`
	MinConfidence                float64 `mapstructure:"min_confidence" yaml:"min_confidence"`
	EncryptContent               bool    `mapstructure:"encrypt_content" yaml:"encrypt_content"`
	ExtractionBatchSize          int     `mapstructure:"extraction_batch_size" yaml:"extraction_batch_size"`
	ExtractionFlushSeconds       int     `mapstructure:"extraction_flush_seconds" yaml:"extraction_flush_seconds"`
	ExtractionContextCharacters  int     `mapstructure:"extraction_context_characters" yaml:"extraction_context_characters"`
	CandidatePoolSize            int     `mapstructure:"candidate_pool_size" yaml:"candidate_pool_size"`
	VectorWeight                 float64 `mapstructure:"vector_weight" yaml:"vector_weight"`
	KeywordWeight                float64 `mapstructure:"keyword_weight" yaml:"keyword_weight"`
	MinCombinedScore             float64 `mapstructure:"min_combined_score" yaml:"min_combined_score"`
	MinKeywordScore              float64 `mapstructure:"min_keyword_score" yaml:"min_keyword_score"`
}

type PromptConfig struct {
	System        string `mapstructure:"system" yaml:"system"`
	IdentityRule  string `mapstructure:"identity_rule" yaml:"identity_rule"`
	Developer     string `mapstructure:"developer" yaml:"developer"`
	Intent        string `mapstructure:"intent" yaml:"intent"`
	IntentReply   string `mapstructure:"intent_reply" yaml:"intent_reply"`
	InterestScore string `mapstructure:"interest_score" yaml:"interest_score"`
	Summary       string `mapstructure:"summary" yaml:"summary"`
}

type SummaryConfig struct {
	Enabled             bool `mapstructure:"enabled" yaml:"enabled"`
	SummaryMessageLimit int  `mapstructure:"summary_message_limit" yaml:"summary_message_limit"`
	MessageThreshold    int  `mapstructure:"message_threshold" yaml:"message_threshold"`
}

var Cfg *Config

func LoadConfig() error {
	if err := checkConfig(); err != nil {
		return err
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath("config/personalities")
	viper.AddConfigPath("config")
	viper.AddConfigPath(".")

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetDefault("platform.replacements.enabled", false)
	viper.SetDefault("ai.interest.enable_interest_detection", false)
	viper.SetDefault("ai.summary.enabled", false)
	viper.SetDefault("ai.memory.enabled", false)
	viper.SetDefault("ai.memory.local_gate_enabled", true)
	viper.SetDefault("ai.memory.local_gate_min_query_tokens", 3)
	viper.SetDefault("ai.memory.self_min_similarity", 0.78)
	viper.SetDefault("ai.memory.self_enabled", false)
	viper.SetDefault("ai.memory.self_extraction_model", "gpt-5-mini")
	viper.SetDefault("ai.memory.self_max_injected", 10)
	viper.SetDefault("ai.memory.self_max_injected_characters", 5000)
	viper.SetDefault("ai.memory.conflict_notifications_enabled", true)
	viper.SetDefault("ai.memory.write_enabled", true)
	viper.SetDefault("ai.memory.retrieval_mode", "off")
	viper.SetDefault("ai.memory.extraction_model", "gpt-5-mini")
	viper.SetDefault("ai.memory.retrieval_gate_model", "gpt-5-mini")
	viper.SetDefault("ai.memory.embedding_model", "text-embedding-3-small")
	viper.SetDefault("ai.memory.embedding_dimensions", 1536)
	viper.SetDefault("ai.memory.max_candidates", 20)
	viper.SetDefault("ai.memory.max_injected", 5)
	viper.SetDefault("ai.memory.max_injected_characters", 3000)
	viper.SetDefault("ai.memory.min_similarity", 0.78)
	viper.SetDefault("ai.memory.min_confidence", 0.65)
	viper.SetDefault("ai.memory.encrypt_content", true)
	viper.SetDefault("ai.memory.extraction_batch_size", 5)
	viper.SetDefault("ai.memory.extraction_flush_seconds", 300)
	viper.SetDefault("ai.memory.extraction_context_characters", 6000)
	viper.SetDefault("ai.memory.candidate_pool_size", 50)
	viper.SetDefault("ai.memory.vector_weight", 0.65)
	viper.SetDefault("ai.memory.keyword_weight", 0.35)
	viper.SetDefault("ai.memory.min_combined_score", 0.35)
	viper.SetDefault("ai.memory.min_keyword_score", 0.20)
	viper.SetDefault("ai.runtime.enable_direct_throttle", true)
	viper.SetDefault("ai.runtime.conversation_ttl_seconds", 21600)
	viper.SetDefault("ai.runtime.max_conversation_mappings", 1000)
	viper.SetDefault("ai.runtime.direct_flow_user_cooldown_seconds", 3)
	viper.SetDefault("ai.runtime.direct_flow_channel_cooldown_seconds", 1)
	viper.SetDefault("ai.runtime.max_direct_limiter_entries", 4000)
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		return err
	}

	personalityFile := viper.GetString("ai.personality")
	if personalityFile != "" {
		viper.SetConfigName(personalityFile)

		if err := viper.MergeInConfig(); err != nil {
			return err
		}
	}

	var cfg Config
	err = viper.Unmarshal(&cfg)
	if err != nil {
		return err
	}
	cfg.AI.Prompts.System = helper.MinifyPrompt(cfg.AI.Prompts.System)
	cfg.AI.Prompts.Developer = helper.MinifyPrompt(cfg.AI.Prompts.Developer)
	cfg.AI.Prompts.IdentityRule = helper.MinifyPrompt(cfg.AI.Prompts.IdentityRule)
	cfg.AI.Prompts.Summary = helper.MinifyPrompt(cfg.AI.Prompts.Summary)
	cfg.AI.Prompts.Intent = helper.MinifyPrompt(cfg.AI.Prompts.Intent)
	cfg.AI.Prompts.IntentReply = helper.MinifyPrompt(cfg.AI.Prompts.IntentReply)
	cfg.AI.Prompts.InterestScore = helper.MinifyPrompt(cfg.AI.Prompts.InterestScore)

	Cfg = &cfg

	return nil
}

func checkConfig() error {
	if _, err := os.Stat("config/config.yml"); err == nil {
		return nil // file already exists
	}

	var cfg Config

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}

	return os.WriteFile("config.yml", data, 0644)
}

func GetConfig() *Config {
	return Cfg
}

// GetEncryptionKey decodes the hex encryption key from config.
// Returns nil if encryption is disabled (empty key).
// Returns an error if the key is set but malformed or not 32 bytes.
func GetEncryptionKey() ([]byte, error) {
	if Cfg == nil || Cfg.Security.EncryptionKey == "" {
		return nil, nil
	}

	key, err := hex.DecodeString(Cfg.Security.EncryptionKey)
	if err != nil {
		return nil, errors.New("security.encryption_key: invalid hex string")
	}

	if len(key) != 32 {
		return nil, errors.New("security.encryption_key: must be 64 hex characters (32 bytes) for AES-256")
	}

	return key, nil
}
