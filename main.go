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
	UserID string `json:"user_id"`
	Data   string `json:"data"`
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

	c.JSON(http.StatusOK, gin.H{"context": data})
}

func updateContext(c *gin.Context) {
	var ctx Context
	if err := c.ShouldBindJSON(&ctx); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	_, err := db.Exec("INSERT INTO contexts (user_id, data) VALUES (?, ?) ON CONFLICT(user_id) DO UPDATE SET data = ?", ctx.UserID, ctx.Data, ctx.Data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update context"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "context updated"})
}

func chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	var context string
	db.QueryRow("SELECT data FROM contexts WHERE user_id = ?", req.UserID).Scan(&context)

	fullPrompt := fmt.Sprintf("Context: %s\nUser: %s\nAI:", context, req.Message)

	reqBody, _ := json.Marshal(map[string]any{
		"model":  MODEL_NAME,
		"prompt": fullPrompt,
		"stream": false,
	})

	resp, err := http.Post(OLLAMA_API_URL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to Ollama"})
		return
	}
	defer resp.Body.Close()

	var ollamaResp map[string]any
	json.NewDecoder(resp.Body).Decode(&ollamaResp)

	c.JSON(http.StatusOK, gin.H{"response": ollamaResp["response"]})
}

func main() {
	initDB()
	r := gin.Default()

	r.GET("/context/:user_id", getContext)
	r.POST("/context", updateContext)
	r.POST("/chat", chat)

	r.Run(":8001")
}
