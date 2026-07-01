package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
	aiHandler "github.com/iotatfan/sora-go/internal/ai"
	"github.com/iotatfan/sora-go/internal/commands"
	"github.com/iotatfan/sora-go/internal/config"
	"github.com/iotatfan/sora-go/internal/models"
	"github.com/iotatfan/sora-go/internal/repository"
	urlReplaceHandler "github.com/iotatfan/sora-go/internal/url_replace"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	ctx := context.Background()

	if err := config.LoadConfig(); err != nil {
		fmt.Println("config load error:", err)
		return
	}

	cfg := config.GetConfig()
	dsn := cfg.Database.DSN
	var userRepo repository.UserRepository
	if dsn == "" {
		fmt.Println("Warning: Database DSN is empty. Proceeding without database.")
	} else {
		db, gormErr := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if gormErr != nil {
			fmt.Println("Warning: failed to connect to database:", gormErr)
			fmt.Println("Proceeding without database support.")
		} else {

			if err := db.AutoMigrate(&models.UserProfile{}); err != nil {
				fmt.Println("Warning: database migration error:", err)
			}
			userRepo = repository.NewUserRepository(db)
		}
	}
	discord, err := discordgo.New("Bot " + cfg.Auth.DiscordToken)
	if err != nil {
		fmt.Println("Error creating discord session,", err)
		return
	}

	aiClient := openai.NewClient(
		option.WithAPIKey(cfg.Auth.OpenAIKey),
	)
	handler := aiHandler.NewAIHandler(cfg, &aiClient, userRepo)

	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handler.ParseMessage(s, m, ctx)
	})
	discord.AddHandler(urlReplaceHandler.ParseUrl)

	if cfg.App.EnableCommands {
		commands.RegisterCommands(discord)
	}

	if err := discord.Open(); err != nil {
		fmt.Println("discord open error:", err)
		return
	}
	defer discord.Close()

	fmt.Println("Started")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
