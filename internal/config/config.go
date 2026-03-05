package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Token              string `mapstructure:"token"`
	OwnerID            string `mapstructure:"owner_id"`
	BotID              string `mapstructure:"bot_id"`
	TwitterReplaceText string `mapstructure:"twitter_replace_text"`
	IGReplaceText      string `mapstructure:"ig_replace_text"`
	OpenAIKey          string `mapstructure:"open_ai_key"`
	GPTSystemPrompt    string `mapstructure:"gpt_system_prompt"`
}

var AppConfig *Config

func LoadConfig() error {
	// viper.SetConfigFile(".env")
	// viper.ReadInConfig()
	// viper.AutomaticEnv()

	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")

	// viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
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

	fmt.Println(cfg)
	AppConfig = &cfg

	return nil
}

func GetConfig() *Config {
	return AppConfig
}
