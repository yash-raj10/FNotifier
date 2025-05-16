package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type form struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Gmail       string `json:"gmail"`
	Description string `json:"description"`
}

type BotInfo struct {
	adminChatID int64
}

var (
	sheetsService *sheets.Service
	serviceReady  = false
	serviceMutex  sync.RWMutex
)

func isServiceReady() bool {
	serviceMutex.RLock()
	defer serviceMutex.RUnlock()
	return serviceReady
}

func setServiceReady(srv *sheets.Service) {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	sheetsService = srv
	serviceReady = true
}

func main() {
	err := godotenv.Load(".env.local")
	if err != nil {
		log.Fatal("Error loading .env file in main")
	}

	// Initialize Telegram bot
	botInfo := &BotInfo{}
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TG_BOT_TOKEN"))
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Start Telegram
	go setupTelegramBot(bot, botInfo)

	// Initialize db
	DB_URL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", DB_URL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS formdata (id SERIAL PRIMARY KEY, name TEXT, gmail TEXT, description TEXT)")
	if err != nil {
		log.Fatal(err)
	}

	r := gin.Default()

	// Setup routes
	setupRoutes(r, db, bot, botInfo)

	// Start GoogleOAuth 
	go setupGoogleOAuth()

	// Start server
	log.Println("Starting server on :8080")
	if err := r.Run(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func setupTelegramBot(bot *tgbotapi.BotAPI, botInfo *BotInfo) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			botInfo.adminChatID = update.Message.Chat.ID
			log.Printf("Set admin chat ID to: %d", botInfo.adminChatID)
		}
	}
}

func setupRoutes(r *gin.Engine, db *sql.DB, bot *tgbotapi.BotAPI, botInfo *BotInfo) {
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "Server is running"})
	})

	// OAuth callback route
	r.GET("/callback", handleOAuthCallback)

	r.POST("/SendForm", func(c *gin.Context) {
		if !isServiceReady() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Google Sheets service is not ready yet. Please try again in a moment.",
			})
			return
		}
		
		handleFormSubmission(c, db, bot, botInfo, sheetsService)
	})

	r.GET("/status", func(c *gin.Context) {
		if isServiceReady() {
			c.JSON(http.StatusOK, gin.H{"status": "Google Sheets service is ready"})
		} else {
			c.JSON(http.StatusOK, gin.H{"status": "Google Sheets service is initializing"})
		}
	})
}


var (
	oauthConfig *oauth2.Config
	oauthToken  *oauth2.Token
	tokenFile   = "token.json"
)

func setupGoogleOAuth() {
	ctx := context.Background()

	b, err := os.ReadFile("web.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
		return
	}

	oauthConfig, err = google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets")
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
		return
	}
	oauthConfig.RedirectURL = "http://localhost:8080/callback"

	client, err := getOAuthClient(ctx)
	if err != nil {
		authURL := oauthConfig.AuthCodeURL("state")
		fmt.Printf("Visit this URL to authorize: %v\n", authURL)
		return
	}

	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
		return
	}

	setServiceReady(srv)
	log.Println("Google Sheets service is now ready")
}

func getOAuthClient(ctx context.Context) (*http.Client, error) {
	tok, err := readTokenFromFile(tokenFile)
	if err == nil {
		return oauthConfig.Client(ctx, tok), nil
	}
	
	return nil, fmt.Errorf("no saved token")
}

func handleOAuthCallback(c *gin.Context) {
	ctx := context.Background()
	code := c.Query("code")
	if code == "" {
		c.String(400, "Code not found")
		return
	}

	tok, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		c.String(500, "Failed to exchange token: "+err.Error())
		return
	}

	if err := saveTokenToFile(tokenFile, tok); err != nil {
		log.Printf("Unable to save token: %v", err)
	}

	client := oauthConfig.Client(ctx, tok)
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		c.String(500, "Failed to create sheets service: "+err.Error())
		return
	}

	setServiceReady(srv)
	log.Println("Google Sheets service is now ready")

	c.String(200, "Authorization successful! You can close this window and use the form now.")
}

func saveTokenToFile(file string, token *oauth2.Token) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

func readTokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func handleFormSubmission(c *gin.Context, db *sql.DB, bot *tgbotapi.BotAPI, botInfo *BotInfo, srv *sheets.Service) {
	name := c.PostForm("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is not provided"})
		return
	}

	gmail := c.PostForm("gmail")
	if gmail == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Gmail is not provided"})
		return
	}

	description := c.PostForm("description")
	if description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Description is not provided"})
		return
	}

	Data := form{
		Name:        name,
		Gmail:       gmail,
		Description: description,
	}

	// DB insertion
	err := db.QueryRow("INSERT INTO formdata (name, gmail, description) VALUES ($1, $2, $3) RETURNING id", 
		Data.Name, Data.Gmail, Data.Description).Scan(&Data.Id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to insert data into database: " + err.Error(),
		})
		return
	}

	// TG sent
	chatID := botInfo.adminChatID
	if chatID != 0 {
		text := fmt.Sprintf("New Contact Request!\n\nName: %s\nEmail: %s\nMessage: %s",
			Data.Name, Data.Gmail, Data.Description)

		msg := tgbotapi.NewMessage(chatID, text)
		_, err := bot.Send(msg)
		if err != nil {
			log.Printf("Failed to send Telegram message: %v", err)
		}
	}

	// Sheet save
	spreadsheetId := "1X5nVMQqSsYuJhxu_DAXuy0ZycKBWgSVJIeBIRZc04KA"
	readRange := "Sheet1!A2:E"

	values := [][]interface{}{{
		Data.Name,
		Data.Gmail,
		Data.Description,
	}}

	valueRange := &sheets.ValueRange{
		Values: values,
	}

	_, err = srv.Spreadsheets.Values.Append(spreadsheetId, readRange, valueRange).
		ValueInputOption("RAW").
		InsertDataOption("INSERT_ROWS").
		Do()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to append data to spreadsheet: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"_id":         Data.Id,
		"name":        Data.Name,
		"email":       Data.Gmail,
		"description": Data.Description,
	})
}