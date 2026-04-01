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
	Replacements ReplacementConfig `mapstructure:"replacements" yaml:"replacements"`
}

type ReplacementConfig struct {
	Twitter   string `mapstructure:"twitter" yaml:"twitter"`
	Instagram string `mapstructure:"instagram" yaml:"instagram"`
}

type AIConfig struct {
	Prompts PromptConfig `mapstructure:"prompts" yaml:"prompts"`
}

type PromptConfig struct {
	System       string `mapstructure:"system" yaml:"system"`
	IdentityRule string `mapstructure:"identity_rule" yaml:"identity_rule"`
	Developer    string `mapstructure:"developer" yaml:"developer"`
	Intent       string `mapstructure:"intent" yaml:"intent"`
	IntentReply  string `mapstructure:"intent_reply" yaml:"intent_reply"`
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
