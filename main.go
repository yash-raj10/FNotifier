package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/oauth2"
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

type Credentials struct {
	Type                string `json:"type"`
	ProjectID           string `json:"project_id"`
	PrivateKeyID        string `json:"private_key_id"`
	PrivateKey          string `json:"private_key"`
	ClientEmail         string `json:"client_email"`
	ClientID            string `json:"client_id"`
	AuthURI             string `json:"auth_uri"`
	TokenURI            string `json:"token_uri"`
	AuthProviderCertURL string `json:"auth_provider_x509_cert_url"`
	ClientCertURL       string `json:"client_x509_cert_url"`
}

func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
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
	//log.Printf("Authorized on account %s", bot.Self.UserName)

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
	client := getClient(config)

	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
	}
	//1X5nVMQqSsYuJhxu_DAXuy0ZycKBWgSVJIeBIRZc04KA

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

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"Update": "working"})
	})
	r.POST("/SendForm", sendHAndler(db, bot, botInfo, srv))
	//r.GET("/getALL", getHandler)

	r.Run()
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
		readRange := "Fnotifier!A2:E"

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
