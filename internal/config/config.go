package config

import (
	"os"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Token              string `mapstructure:"token" yaml:"token"`
	OwnerID            string `mapstructure:"owner_id" yaml:"owner_id"`
	BotID              string `mapstructure:"bot_id" yaml:"bot_id"`
	TwitterReplaceText string `mapstructure:"twitter_replace_text" yaml:"twitter_replace_text"`
	IGReplaceText      string `mapstructure:"ig_replace_text" yaml:"ig_replace_text"`
	OpenAIKey          string `mapstructure:"open_ai_key" yaml:"open_ai_key"`
	GPTSystemPrompt    string `mapstructure:"gpt_system_prompt" yaml:"gpt_system_prompt"`
}

var AppConfig *Config

func LoadConfig() error {
	checkConfig()

	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")

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

	AppConfig = &cfg

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
	return AppConfig
}
