package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/src"
)

var (
	conf *src.Config
)

func main() {
	conf = src.LoadConfig("config.json")

	discord, err := discordgo.New("Bot " + conf.BotToken)
	if err != nil {
		fmt.Println("Error creating discord session,", err)
		return
	}

	discord.AddHandler(src.ParseUrl)

	discord.Open()
	defer discord.Close()

	fmt.Println("Started")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
