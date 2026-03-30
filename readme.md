# Ama3 Bot

A Discord bot written in Go, integrating OpenAI via openai-go with conversational context handling and model fallback.

## Overview
Ama3-bot enables AI-driven conversations inside Discord.

- Start a new conversation by tagging the bot  
- Continue conversation by replying to bot messages  

Conversation context is stored in memory, allowing short-lived session continuity without external storage.

## Features
- AI-powered responses via OpenAI
- Tag-to-start conversation flow
- Reply-based conversation continuation
- In-memory conversation tracking
- Primary + fallback model strategy
- Concurrent request handling (Go)

## Conversation Flow

1. **Start**
   - Mention/tag the bot → creates new conversation session  

2. **Continue**
   - Reply to bot message → continues existing session  

3. **Context Handling**
   - Conversation history stored in memory  
   - Context persists during runtime only  

## Model Strategy

- **Primary:** GPT-5.2  
- **Fallback:** GPT-5.1 Mini  

Fallback ensures response continuity when primary fails or is unavailable.

## Tech Stack
- Go
- discordgo
- openai-go
