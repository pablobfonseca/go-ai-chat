package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/sqlite"
)

const OLLAMA_API_URL = "http://localhost:11434/api/generate"
const MODEL_NAME = "mistral"

type Context struct {
	UserID   string    `json:"user_id"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "context.db")
	if err != nil {
		panic(err)
	}
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS contexts (user_id TEXT PRIMARY KEY, data TEXT)")
	if err != nil {
		panic(err)
	}
}

func getContext(c *gin.Context) {
	userID := c.Param("user_id")
	var data string
	err := db.QueryRow("SELECT data FROM contexts WHERE user_id = ?", userID).Scan(&data)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"context": "{}"})
		return
	}

	var storedContext Context
	json.Unmarshal([]byte(data), &storedContext)
	c.JSON(http.StatusOK, gin.H{"context": storedContext})
}

func updateContext(c *gin.Context) {
	var ctx Context
	if err := c.ShouldBindJSON(&ctx); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	data, _ := json.Marshal(ctx)
	_, err := db.Exec("INSERT INTO contexts (user_id, data) VALUES (?, ?) ON CONFLICT(user_id) DO UPDATE SET data = ?", ctx.UserID, string(data), string(data))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update context"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Context updated"})
}

func chat(c *gin.Context) {
	userID := c.Query("user_id")
	message := c.Query("message")

	if userID == "" || message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing user_id or message"})
		return
	}

	var storedContext Context
	var data string
	err := db.QueryRow("SELECT data FROM contexts WHERE user_id = ?", userID).Scan(&data)
	if err == nil {
		json.Unmarshal([]byte(data), &storedContext)
	} else {
		storedContext = Context{UserID: userID, Messages: []Message{}}
	}

	storedContext.Messages = append(storedContext.Messages, Message{Role: "user", Content: message})

	fullPrompt := "The following is a conversation history. Respond based only on the given context using natural language.\n"
	for _, msg := range storedContext.Messages {
		fullPrompt += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
	}
	fullPrompt += "AI:"

	reqBody, _ := json.Marshal(map[string]any{
		"model":  MODEL_NAME,
		"prompt": fullPrompt,
		"stream": true,
	})

	resp, err := http.Post(OLLAMA_API_URL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to Ollama"})
		return
	}
	defer resp.Body.Close()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	decoder := json.NewDecoder(resp.Body)
	var response string
	for decoder.More() {
		var chunk map[string]any
		if err := decoder.Decode(&chunk); err != nil {
			break
		}
		if text, ok := chunk["response"].(string); ok {
			response += text
			fmt.Fprintf(c.Writer, "data: %s\n\n", text)
			flusher.Flush()
		}
	}

	storedContext.Messages = append(storedContext.Messages, Message{Role: "ai", Content: response})
	updatedData, _ := json.Marshal(storedContext)
	_, err = db.Exec("INSERT INTO contexts (user_id, data) VALUES (?, ?) ON CONFLICT(user_id) DO UPDATE SET data = ?", userID, string(updatedData), string(updatedData))
	if err != nil {
		fmt.Println("Failed to update context in DB: ", err)
	}
}

func main() {
	initDB()
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})
	r.GET("/context/:user_id", getContext)
	r.POST("/context", updateContext)
	r.GET("/chat", chat)

	r.Run(":8001")
}
