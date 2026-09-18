# Ama3

Ama3 is a high-performance Discord bot written in **Go**, designed for sophisticated, context-aware interactions. Ama3 uses context-aware responses, local noise detection, and persistent summaries to provide an administrative AI experience inspired by the *Arknights* "Kal'tsit" persona.

---

## Core Features

### Intelligence & Conversation
* **Local Noise Detection:** Filters obvious greetings, reactions, and punctuation locally before spending model quota.
* **Persistent User Summaries:** Utilizes a PostgreSQL backend to maintain long-term summaries of users, tracking interaction history, technical interests, and behavioral traits.
* **Visual Awareness:** Enriches message context with attachment and embed metadata, allowing the AI to "see" and analyze images (e.g., Gunpla reviews or technical specifications) even with minimal text.
* **Autonomous Interjections:** Analyzes ongoing chat; if a message passes a specific "interest threshold" related to high-level topics (Biology, Engineering, Strategy), the bot will "overhear" and contribute.

### Reliability & Performance
* **Request Protection:** Shared concurrency, spacing, and quota pause controls prevent request bursts and duplicate retries.
* **Fluid UX:** Real-time typing indicators and sentence-aware chunking to bypass Discord's 2000-character limit seamlessly.
* **Direct Flow Throttling:** Configurable limiters to prevent API abuse and maintain conversational focus.

---

## Architecture Snapshot

| Module | Responsibility |
| :--- | :--- |
| `internal/ai/handler.go` | **The Brain:** Orchestrates flow from Discord events to AI responses. |
| `internal/ai/intent.go` | **Conversation Context:** Builds history and optionally scores autonomous interjections. |
| `internal/ai/state.go` | **Memory Management:** Handles in-memory conversation maps and cooldowns. |
| `internal/models/` | **Persistence:** Defines `UserProfile` entities for database storage. |

---

## Getting Started

### Prerequisites
* **Go** `1.22.3+`
* **PostgreSQL** (for user summary)
* **Discord** Bot Token & ID
* **OpenAI** API Key

### Installation
1.  **Clone the repository.**
2.  **Generate the default configuration:**
    ```bash
    go run .
    ```
3.  **Configure credentials:** Edit the generated `config/config.yml` with your Discord tokens, OpenAI key, and database connection string.

### Minimal Configuration
```yaml
# ── Identity ────────────────────────────────────────────────────────────────
app:
  owner_id: "YOUR_DISCORD_USER_ID"   # Your personal Discord user ID (grants doctor-level access)
  bot_id:   "YOUR_BOT_USER_ID"       # The bot application's user ID
  role_id:  ""                        # Optional: role ID for role-gated commands
  enable_commands: false              # Set true to register slash commands

# ── Credentials ─────────────────────────────────────────────────────────────
auth:
  discord_token: "YOUR_DISCORD_BOT_TOKEN"
  openai_key:    "YOUR_OPENAI_API_KEY"

# ── Database (required for summaries) ───────────────────────────────────────
database:
  dsn: "postgres://user:pass@localhost:5432/ama3?sslmode=disable"
  max_open_conns:    25
  max_idle_conns:    5
  conn_max_lifetime: "15m"

# ── AI: Runtime ─────────────────────────────────────────────────────────────
ai:
  personality: "example"              # Load a specific personality from config/personalities/
  runtime:
    enable_direct_throttle: true      # Rate-limit direct-mention flows
    conversation_ttl_seconds: 21600   # How long idle conversation context is kept (6 h)
    max_conversation_mappings: 1000   # Max concurrent in-memory conversation maps
    direct_flow_user_cooldown_seconds:    3
    direct_flow_channel_cooldown_seconds: 1
    max_direct_limiter_entries: 4000
    model_max_concurrent: 2
    model_min_interval_ms: 250
    model_quota_cooldown_seconds: 60
    noise_reaction_probability: 0.70
    noise_response_model: "gpt-5.4-mini"

  # ── AI: Autonomous Interjections ──────────────────────────────────────────
  interest:
    enable_interest_detection: false  # Set true to allow the bot to "overhear" chats
    interest_score_threshold:  0.7    # Minimum score (0.0–1.0) required to interject
    past_message_limit:        15     # How many messages are sampled for scoring
    cooldown_seconds:          600    # Minimum gap between interjections (10 min)

  # ── AI: Summaries ─────────────────────────────────────────────────────────
  summary:
    enabled:               false  # Set true to periodically compress user history
    message_threshold:     30     # New messages before a summary is triggered
    summary_message_limit: 30     # Messages fed into each summary pass

  memory:
    enable_embeddings: true       # Use vector embeddings; false uses keyword retrieval and no embedding API calls
    embedding_model: "text-embedding-3-small"
    embedding_dimensions: 1536
    embedding_cache_max_entries: 2048
    embedding_cache_ttl_seconds: 3600
    local_gate_enabled: true      # Avoid embedding queries for ordinary messages
    retrieval_mode: always        # Avoid the extra model call used by retrieval_mode: model

# ── Security ────────────────────────────────────────────────────────────────
security:
  # AES-256 key for at-rest encryption of user summary data.
  # Must be exactly 64 hex characters (32 bytes).
  # Generate with: openssl rand -hex 32
  # Leave empty to store summaries as plaintext.
  encryption_key: ""
```

---

# Advanced Behavior

Ama3 is highly tunable via `config/config.yml` or Viper environment variables (use `_` in place of `.`, e.g. `AI_RUNTIME_ENABLE_DIRECT_THROTTLE=true`).

| Key | Description |
| :--- | :--- |
| `ai.personality` | Loads personality-specific configuration from `config/personalities/<personality>.yml`. |
| `ai.prompts.*` | Override the built-in system, developer, interest, and summary prompt templates. |
| `ai.interest.interest_score_threshold` | Tune how aggressively the bot interjects into conversations. |
| `security.encryption_key` | Enable AES-256-GCM at-rest encryption for stored user summaries. |

---

# Prompt System (`ai.prompts`)

### `system`
Character identity and behavioral ruleset injected as the OpenAI `system` role. Defines persona, tone protocols, rejection logic, and surveillance access rules.

| Variable | Type | Description |
| :--- | :--- | :--- |
| `{{.OwnerID}}` | `string` | Discord user ID of the bot owner (mention formatting and log mapping). |
| `{{.BotID}}` | `string` | Discord user ID of the bot itself (log mapping). |

### `developer`
Output formatting and safety constraints injected as the OpenAI `developer` role. Controls response density, mention protocol, language switching, and PII handling.

| Variable | Type | Description |
| :--- | :--- | :--- |
| `{{.OwnerID}}` | `string` | Discord user ID of the bot owner (correct mention format in output rules). |

### `interest_score`
Scores a message transcript to decide whether the bot should autonomously interject. The result is compared against `ai.interest.interest_score_threshold`.

| Variable | Type | Description |
| :--- | :--- | :--- |
| `{{.OwnerID}}` | `string` | Discord user ID of the owner (score bonuses/penalties). |
| `{{.UserSummary}}` | `string` | Summary of the message sender. |
| `{{.History}}` | `string` | Recent transcript being evaluated. |
| `{{.Message}}` | `string` | The latest message that triggered scoring. |

**Returns:** A single `float64` in the range `0.0 – 1.0`. Any other output suppresses the interjection.

### `summary`
Compresses a user's conversation history into a fixed-capacity summary string stored in the database.

| Variable | Type | Description |
| :--- | :--- | :--- |
| `{{.Username}}` | `string` | Display name of the user being summarized. |
| `{{.OldSummary}}` | `string` | The existing compressed summary (may be empty on first run). |
| `{{.NewMessages}}` | `string` | New log entries since the last summary pass. |

**Returns:** Plain text ≤ 3000 characters. Stored verbatim and injected as `{{.UserSummary}}` in all other prompts.
