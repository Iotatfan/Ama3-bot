package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
	aiHandler "github.com/iotatfan/sora-go/internal/ai"
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

	dsn := config.GetConfig().Database.DSN
	var userRepo repository.UserRepository
	if dsn == "" {
		fmt.Println("Warning: Database DSN is empty. Proceeding without database.")
	} else {
		db, gormErr := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if gormErr != nil {
			fmt.Println("failed to connect to database:", gormErr)
			return
		}

		if err := db.AutoMigrate(&models.UserProfile{}); err != nil {
			fmt.Println("Error during database migration:", err)
		}
		userRepo = repository.NewUserRepository(db)
	}
	handler := aiHandler.NewAIHandler(userRepo)

	discord, err := discordgo.New("Bot " + config.GetConfig().Auth.DiscordToken)
	if err != nil {
		fmt.Println("Error creating discord session,", err)
		return
	}

	aiClient := openai.NewClient(
		option.WithAPIKey(config.GetConfig().Auth.OpenAIKey),
	)

	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handler.ParseMessage(s, m, &aiClient, ctx)
	})
	discord.AddHandler(urlReplaceHandler.ParseUrl)
	// commands.RegisterCommands(discord)

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
