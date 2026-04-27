package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// Configurations
var (
	jwtKey       []byte
	adminUser    string
	adminHash    []byte
	messagesFile = "messages.json"
	mu           sync.Mutex
	db           *sql.DB
	upgrader     = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clients = make(map[*websocket.Conn]bool)
	broadcast = make(chan ChatMessage)
)

type ChatMessage struct {
	Sender  string `json:"sender"`
	Message string `json:"message"`
	Time     time.Time `json:"time"`
}

type InternalEmail struct {
	ID      int       `json:"id"`
	From    string    `json:"from"`
	Subject string    `json:"subject"`
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
}

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "svv-portfolio-secure-random-key-2026"
		log.Println("WARNING: JWT_SECRET not set, using default.")
	}
	jwtKey = []byte(secret)

	adminUser = os.Getenv("ADMIN_USER")
	if adminUser == "" {
		adminUser = "Surya"
	}

	pass := os.Getenv("ADMIN_PASS")
	if pass == "" {
		pass = "suryavamsi"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to generate admin hash: %v", err)
	}
	adminHash = hash

	// Initialize Database if URL is provided
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		var err error
		db, err = sql.Open("postgres", dbURL)
		if err != nil {
			log.Printf("Failed to connect to database: %v. Falling back to JSON.", err)
		} else {
			err = db.Ping()
			if err != nil {
				log.Printf("Database ping failed: %v. Falling back to JSON.", err)
				db = nil
			} else {
				log.Println("Connected to PostgreSQL database.")
				createTables()
				seedAdmin()
			}
		}
	} else {
		log.Println("No DATABASE_URL found. Using messages.json for storage.")
	}
}

func createTables() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT DEFAULT 'user'
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id SERIAL PRIMARY KEY,
			name TEXT,
			email TEXT,
			phone TEXT,
			company TEXT,
			region TEXT,
			country TEXT,
			message TEXT,
			time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS internal_emails (
			id SERIAL PRIMARY KEY,
			sender TEXT,
			subject TEXT,
			content TEXT,
			time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id SERIAL PRIMARY KEY,
			sender TEXT,
			message TEXT,
			time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
	}
	for _, q := range queries {
		_, err := db.Exec(q)
		if err != nil {
			log.Fatalf("Failed to execute migration: %v", err)
		}
	}
}

func seedAdmin() {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		log.Printf("Error checking users count: %v", err)
		return
	}

	if count == 0 {
		pass := os.Getenv("ADMIN_PASS")
		if pass == "" {
			pass = "suryavamsi"
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
		_, err = db.Exec("INSERT INTO users (username, password_hash, role) VALUES ($1, $2, $3)", adminUser, string(hash), "admin")
		if err != nil {
			log.Printf("Failed to seed admin: %v", err)
		} else {
			log.Println("Admin user seeded in database.")
		}
	}
}

type Message struct {
	ID      int       `json:"id,omitempty"`
	Name    string    `json:"name"`
	Email   string    `json:"email"`
	Phone   string    `json:"phone"`
	Company string    `json:"company"`
	Region  string    `json:"region"`
	Country string    `json:"country"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Middleware: Authenticate JWT
func authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("token")
		if err != nil {
			if err == http.ErrNoCookie {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		tokenStr := cookie.Value
		claims := &Claims{}

		tkn, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})

		if err != nil {
			if err == jwt.ErrSignatureInvalid {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if !tkn.Valid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var creds Credentials
	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check against database users if DB is available
	var storedHash string
	var role string
	authenticated := false

	if db != nil {
		err := db.QueryRow("SELECT password_hash, role FROM users WHERE username = $1", creds.Username).Scan(&storedHash, &role)
		if err == nil {
			if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(creds.Password)) == nil {
				authenticated = true
			}
		}
	} else {
		// Fallback to Env-based Admin
		if creds.Username == adminUser && bcrypt.CompareHashAndPassword(adminHash, []byte(creds.Password)) == nil {
			authenticated = true
			role = "admin"
		}
	}

	if !authenticated {
		log.Printf("Failed login attempt for user: %s", creds.Username)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	log.Printf("Successful login for user: %s (Role: %s)", creds.Username, role)

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: creds.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    tokenString,
		Expires:  expirationTime,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // Enforce HTTPS in production
		SameSite: http.SameSiteLaxMode,
	})

	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func handleGetMessages(w http.ResponseWriter, r *http.Request) {
	var messages []Message

	if db != nil {
		rows, err := db.Query("SELECT id, name, email, phone, company, region, country, message, time FROM messages ORDER BY time DESC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var m Message
			err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Phone, &m.Company, &m.Region, &m.Country, &m.Message, &m.Time)
			if err != nil {
				log.Printf("Error scanning row: %v", err)
				continue
			}
			messages = append(messages, m)
		}
	} else {
		mu.Lock()
		data, err := os.ReadFile(messagesFile)
		mu.Unlock()
		if err == nil {
			json.Unmarshal(data, &messages)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func handleContact(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var msg Message
	json.NewDecoder(r.Body).Decode(&msg)
	msg.Time = time.Now()

	if db != nil {
		query := `INSERT INTO messages (name, email, phone, company, region, country, message, time) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
		_, err := db.Exec(query, msg.Name, msg.Email, msg.Phone, msg.Company, msg.Region, msg.Country, msg.Message, msg.Time)
		if err != nil {
			log.Printf("Database error saving message: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		mu.Lock()
		defer mu.Unlock()

		var messages []Message
		data, _ := os.ReadFile(messagesFile)
		json.Unmarshal(data, &messages)
		messages = append(messages, msg)

		finalData, _ := json.MarshalIndent(messages, "", "  ")
		os.WriteFile(messagesFile, finalData, 0644)
	}

	log.Printf("Received transmission from %s (%s)", msg.Name, msg.Email)
	w.WriteHeader(http.StatusOK)
}

func handleUpdateMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		return
	}

	var req struct {
		ID    int `json:"id"`
		Index int `json:"index"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if db != nil {
		_, err := db.Exec("DELETE FROM messages WHERE id = $1", req.ID)
		if err != nil {
			log.Printf("Database error deleting message: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		mu.Lock()
		defer mu.Unlock()

		var messages []Message
		data, _ := os.ReadFile(messagesFile)
		json.Unmarshal(data, &messages)

		if req.Index >= 0 && req.Index < len(messages) {
			messages = append(messages[:req.Index], messages[req.Index+1:]...)
			finalData, _ := json.MarshalIndent(messages, "", "  ")
			os.WriteFile(messagesFile, finalData, 0644)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade error: %v", err)
		return
	}
	defer conn.Close()

	mu.Lock()
	clients[conn] = true
	mu.Unlock()

	for {
		var msg ChatMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			mu.Lock()
			delete(clients, conn)
			mu.Unlock()
			break
		}
		msg.Time = time.Now()
		broadcast <- msg
	}
}

func handleMessages() {
	for {
		msg := <-broadcast
		if db != nil {
			db.Exec("INSERT INTO chat_messages (sender, message, time) VALUES ($1, $2, $3)", msg.Sender, msg.Message, msg.Time)
		}
		mu.Lock()
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				client.Close()
				delete(clients, client)
			}
		}
		mu.Unlock()
	}
}

func handleGetInternalEmails(w http.ResponseWriter, r *http.Request) {
	var emails []InternalEmail
	if db != nil {
		rows, err := db.Query("SELECT id, sender, subject, content, time FROM internal_emails ORDER BY time DESC LIMIT 20")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var e InternalEmail
				rows.Scan(&e.ID, &e.From, &e.Subject, &e.Content, &e.Time)
				emails = append(emails, e)
			}
		}
	}
	json.NewEncoder(w).Encode(emails)
}

func main() {
	go handleMessages()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "infoAiedge.html")
			return
		}
		if r.URL.Path == "/nexus" {
			http.ServeFile(w, r, "nexus_app.html")
			return
		}
		http.FileServer(http.Dir(".")).ServeHTTP(w, r)
	})

	http.HandleFunc("/ws", handleWS)
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/contact", handleContact)
	http.HandleFunc("/api/messages", authenticate(handleGetMessages))
	http.HandleFunc("/api/employee/emails", authenticate(handleGetInternalEmails))
	http.HandleFunc("/api/messages/update", authenticate(handleUpdateMessages))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8001"
	}

	log.Printf("SVV Portfolio Backend starting on port %s", port)
	serverURL := fmt.Sprintf(":%s", port)
	if err := http.ListenAndServe(serverURL, nil); err != nil {
		log.Fatalf("Error starting server: %s", err)
	}
}

