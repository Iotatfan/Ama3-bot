package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/commands"
	"github.com/iotatfan/sora-go/internal/config"
	urlParser "github.com/iotatfan/sora-go/internal/parser"
	"github.com/spf13/viper"
)

func main() {

	config.LoadConfig()

	discord, err := discordgo.New("Bot " + viper.GetString("TOKEN"))
	if err != nil {
		fmt.Println("Error creating discord session,", err)
		return
	}

	discord.AddHandler(urlParser.ParseUrl)
	commands.RegisterCommands(discord)

	discord.Open()
	defer discord.Close()

	fmt.Println("Started")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
