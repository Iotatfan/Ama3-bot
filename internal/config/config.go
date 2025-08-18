package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Prefix       string `json:"prefix"`
	BotToken     string `json:"bot_token"`
	OwnerId      string `json:"owner_id"`
	SkipServerId string `json:"skip_server"`
}

func LoadConfig() {
	viper.SetConfigFile("ENV")
	viper.ReadInConfig()
	viper.AutomaticEnv()
}
