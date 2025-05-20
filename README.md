# Form Data Automation Tool

This is a lightweight automation tool(like Zapier) built with Go that handles form submissions and distributes the data to:
- PostgreSQL database
- Telegram bot notifications
- Google Sheets integration

It supports Google OAuth2 for Google Sheets access, uses Gin for the HTTP server, and makes use of Go's concurrency features to initialize services in the background.

## Features

- **Multi-channel Distribution**: Form data is simultaneously sent to a database, Telegram, and Google Sheets
- **Concurrent Processing**: Leverages Go's goroutines for efficient handling of requests
- **OAuth Integration**: Secure Google Sheets API authentication
- **RESTful API**: Simple endpoints for form submission and service status
- **Real-time Notifications**: Instant Telegram alerts for new form submissions

## Architecture

This application uses:
- [Gin Web Framework](https://github.com/gin-gonic/gin) for HTTP routing
- [Telegram Bot API](https://github.com/go-telegram-bot-api/telegram-bot-api) for notifications
- [Google Sheets API](https://developers.google.com/sheets/api) for spreadsheet integration
- PostgreSQL for persistent data storage
- Concurrency with goroutines for improved performance

## Prerequisites

- Go 1.18+
- PostgreSQL database
- Telegram Bot Token
- Google Cloud Platform credentials for Sheets API

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/form-automation-tool.git
   cd form-automation-tool
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Create `.env.local` file with necessary environment variables:
   ```
   DB_URL=postgres://username:password@localhost:5432/dbname
   TG_BOT_TOKEN=your_telegram_bot_token
   ```

4. Add your Google OAuth credentials file as `web.json` in the project root directory.

## Usage

1. Start the server:
   ```bash
   go run main.go
   ```

2. On first run, follow the OAuth authorization link displayed in the console to authorize Google Sheets access.

3. Send form data to the application:
   ```bash
   curl -X POST http://localhost:8080/SendForm \
     -d "name=John Doe" \
     -d "gmail=john@example.com" \
     -d "description=This is a test message"
   ```

4. Check application status:
   ```bash
   curl http://localhost:8080/status
   ```

## Telegram Bot Setup

1. Start a chat with your bot
2. Send any message to the bot to establish the admin chat ID
3. The bot will automatically use this chat for notifications

## Configuration

- **PostgreSQL**: Configure connection string via the `DB_URL` environment variable
- **Telegram**: Set your bot token via the `TG_BOT_TOKEN` environment variable
- **Google Sheets**: 
  - Create OAuth credentials in the Google Cloud Console
  - Download and save as `web.json` in the project root
  - Set the target spreadsheet ID in the code.
