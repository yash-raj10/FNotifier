package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"log"
	"net/http"
	"os"
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

func main() {
	botInfo := &BotInfo{}

	// Bot SetUP
	err := godotenv.Load(".env.local")
	if err != nil {
		log.Fatal("Error loading .env file in main")
	}
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TG_BOT_TOKEN"))
	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	// goroutine to handle updates
	go func() {
		for update := range updates {
			if update.Message != nil {
				botInfo.adminChatID = update.Message.Chat.ID
			}
		}
	}()

	//	Spreadsheet config
	ctx := context.Background()
	b, err := os.ReadFile("web.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying, delete previously saved token.json.
	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets")
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	config.RedirectURL = "http://localhost:8080/callback"

	// Create a channel to receive the auth client
	clientChan := make(chan *http.Client)

	//DB setup
	DB_URL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", DB_URL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE  IF NOT EXISTS formdata (id SERIAL PRIMARY KEY, name TEXT, gmail TEXT, description TEXT)")
	if err != nil {
		log.Fatal(err)
	}

	//Gin SetUp
	r := gin.Default()

	r.GET("/callback", func(c *gin.Context) {
		code := c.Query("code")
		if code == "" {
			c.String(400, "Code not found")
			return
		}

		tok, err := config.Exchange(ctx, code)
		if err != nil {
			c.String(500, "Failed to exchange token")
			return
		}

		client := config.Client(ctx, tok)
		clientChan <- client // Send client through channel
		c.String(200, "Authorization successful! You can close this window.")
	})

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"Update": "working"})
	})

	// Start server in a goroutine
	go func() {
		r.Run()
	}()

	// Start OAuth flow
	authURL := config.AuthCodeURL("state")
	fmt.Printf("Visit this URL to authorize: %v\n", authURL)

	// Wait for client from callback
	client := <-clientChan

	// Now create sheets service
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
	}

	r.POST("/SendForm", sendHAndler(db, bot, botInfo, srv))
	//r.GET("/getALL", getHandler)

	select {}
}

func sendHAndler(db *sql.DB, bot *tgbotapi.BotAPI, botInfo *BotInfo, srv *sheets.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.PostForm("name")
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Name is not provided",
			})
			return
		}

		gmail := c.PostForm("gmail")
		if gmail == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "gmail is not provided",
			})
			return
		}

		description := c.PostForm("description")
		if description == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "description is not provided",
			})
			return
		}

		Data := form{
			Name:        name,
			Gmail:       gmail,
			Description: description,
		}

		//DB insertion
		err := db.QueryRow("INSERT INTO formdata (name, gmail, description) VALUES ($1, $2, $3) RETURNING id", Data.Name, Data.Gmail, Data.Description).Scan(&Data.Id)
		if err != nil {
			log.Fatal(err)
		}
		chatID := botInfo.adminChatID

		// TG sent
		if chatID != 0 {
			text := fmt.Sprintf("New Contact Request!\n\nName: %s\nEmail: %s\nMessage: %s",
				Data.Name, Data.Gmail, Data.Description)

			msg := tgbotapi.NewMessage(chatID, text)
			bot.Send(msg)
		}

		//	Sheet save
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

		_, err = srv.Spreadsheets.Values.Append(spreadsheetId, readRange, valueRange).ValueInputOption("RAW").InsertDataOption("INSERT_ROWS").Do()
		if err != nil {
			log.Fatalf("Unable to Append data from sheet: %v", err)
		}

		c.JSON(http.StatusOK, gin.H{
			"_id":         Data.Id,
			"name":        Data.Name,
			"email":       Data.Gmail,
			"description": Data.Description,
		})
	}
}

func getHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {

	}
}
