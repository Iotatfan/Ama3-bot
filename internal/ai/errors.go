package ai

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/iotatfan/sora-go/internal/errorhandler"
)

func (h *AIHandler) sendOpenAIError(discord *discordgo.Session, message *discordgo.MessageCreate, err error) {
	userMessage := errorhandler.OpenAIErrorMessage(err)
	if userMessage == "" || discord == nil || message == nil {
		return
	}

	if _, sendErr := discord.ChannelMessageSendReply(message.ChannelID, userMessage, message.Reference()); sendErr != nil {
		fmt.Println("failed to send OpenAI error message:", sendErr)
	}
}
