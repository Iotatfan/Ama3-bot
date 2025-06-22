package src

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Prefix       string `json:"prefix"`
	BotToken     string `json:"bot_token"`
	OwnerId      string `json:"owner_id"`
	SkipServerId string `json:"skip_server"`
}

func LoadConfig(filename string) *Config {
	body, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("error loading config,", err)
		return nil
	}
	var conf Config
	json.Unmarshal(body, &conf)
	return &conf
}
