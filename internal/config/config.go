package config

import (
	"encoding/hex"
	"errors"
	"os"
	"strings"

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
}

type SecurityConfig struct {
	// EncryptionKey is a 64-character hex string (32 bytes) used for AES-256-GCM
	// encryption of sensitive fields at rest. Leave empty to disable encryption.
	EncryptionKey string `mapstructure:"encryption_key" yaml:"encryption_key"`
}

type AppConfig struct {
	BotID   string `mapstructure:"bot_id" yaml:"bot_id"`
	OwnerID string `mapstructure:"owner_id" yaml:"owner_id"`
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
	BlacklistGuilds []string          `mapstructure:"blacklist_guilds" yaml:"blacklist_guilds"`
	Replacements    ReplacementConfig `mapstructure:"replacements" yaml:"replacements"`
}

type ReplacementConfig struct {
	Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
	Twitter   string `mapstructure:"twitter" yaml:"twitter"`
	Instagram string `mapstructure:"instagram" yaml:"instagram"`
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
	Prompts  PromptConfig   `mapstructure:"prompts" yaml:"prompts"`
	Interest InterestConfig `mapstructure:"interest" yaml:"interest"`
	Runtime  RuntimeConfig  `mapstructure:"runtime" yaml:"runtime"`
	Summary  SummaryConfig  `mapstructure:"summary" yaml:"summary"`
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
	viper.AddConfigPath(".")

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
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

	var cfg Config
	err = viper.Unmarshal(&cfg)
	if err != nil {
		return err
	}

	Cfg = &cfg

	return nil
}

func checkConfig() error {
	if _, err := os.Stat("config.yml"); err == nil {
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
