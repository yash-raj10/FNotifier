package main

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
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
	r.POST("/SendForm", sendHAndler(db, bot, botInfo))
	//r.GET("/getALL", getHandler)

	r.Run()
}

func sendHAndler(db *sql.DB, bot *tgbotapi.BotAPI, botInfo *BotInfo) gin.HandlerFunc {
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

		err := db.QueryRow("INSERT INTO formdata (name, gmail, description) VALUES ($1, $2, $3) RETURNING id", Data.Name, Data.Gmail, Data.Description).Scan(&Data.Id)
		if err != nil {
			log.Fatal(err)
		}
		chatID := botInfo.adminChatID

		if chatID != 0 {
			text := fmt.Sprintf("New Contact Request!\n\nName: %s\nEmail: %s\nMessage: %s",
				Data.Name, Data.Gmail, Data.Description)

			msg := tgbotapi.NewMessage(chatID, text)
			bot.Send(msg)
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
