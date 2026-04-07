package config

import (
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
}

type AppConfig struct {
	BotID   string `mapstructure:"bot_id" yaml:"bot_id"`
	OwnerID string `mapstructure:"owner_id" yaml:"owner_id"`
}

type AuthConfig struct {
	DiscordToken string `mapstructure:"discord_token" yaml:"discord_token"`
	OpenAIKey    string `mapstructure:"openai_key" yaml:"openai_key"`
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

type AIConfig struct {
	Prompts  PromptConfig   `mapstructure:"prompts" yaml:"prompts"`
	Interest InterestConfig `mapstructure:"interest" yaml:"interest"`
}

type PromptConfig struct {
	System        string `mapstructure:"system" yaml:"system"`
	IdentityRule  string `mapstructure:"identity_rule" yaml:"identity_rule"`
	Developer     string `mapstructure:"developer" yaml:"developer"`
	Intent        string `mapstructure:"intent" yaml:"intent"`
	IntentReply   string `mapstructure:"intent_reply" yaml:"intent_reply"`
	InterestScore string `mapstructure:"interest_score" yaml:"interest_score"`
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
