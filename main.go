package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

var BuildNumber = "1"

type Server struct {
	Addr   string
	Domain string
	Active bool
}

type DomainReport struct {
	Domain         string    `json:"domain"`
	TotalRequests  int       `json:"total_requests"`
	MostRecentTime time.Time `json:"most_recent_time"`
}

type DomainConfig struct {
	Domain  string   `json:"domain"`
	Servers []string `json:"servers"`
}

var db *sqlx.DB
var servers map[string][]Server
var current int
var configMutex = &sync.Mutex{}

func main() {
	var err error
	db, err = sqlx.Connect("sqlite3", "./loadbalancer.db")
	if err != nil {
		log.Fatalln(err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS logs (
		request_id TEXT,
		request_ip TEXT,
		user_agent TEXT,
		server_addr TEXT,
		domain TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		http_response INTEGER
	);`)
	if err != nil {
		log.Fatalln(err)
	}

	loadConfig()

	go func() {
		for range time.Tick(60 * time.Second) {
			loadConfig()
		}
	}()

	http.HandleFunc("/", proxyHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/report", reportHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func loadConfig() {
	configMutex.Lock()
	defer configMutex.Unlock()

	data, err := os.ReadFile("./config/domains.json")
	if err != nil {
		log.Println("Error reading domains.json:", err)
		return
	}

	var config []DomainConfig
	err = json.Unmarshal(data, &config)
	if err != nil {
		log.Println("Error parsing domains.json:", err)
		return
	}

	servers = make(map[string][]Server)
	for _, domainConfig := range config {
		for _, serverAddr := range domainConfig.Servers {
			servers[domainConfig.Domain] = append(servers[domainConfig.Domain], Server{
				Addr:   serverAddr,
				Domain: domainConfig.Domain,
				Active: true,
			})
		}
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.Lock()
	defer configMutex.Unlock()

	domainServers, ok := servers[r.Host]
	if !ok || len(domainServers) == 0 {
		http.Error(w, "No servers available for domain", http.StatusInternalServerError)
		return
	}

	server := domainServers[current]
	current = (current + 1) % len(domainServers)

	target, err := url.Parse(server.Addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = logResponse
	proxy.ServeHTTP(w, r)
}

func logResponse(r *http.Response) error {
	requestID := generateHash(32)
	requestIP := strings.Split(r.Request.RemoteAddr, ":")[0]
	userAgent := r.Request.UserAgent()
	serverAddr := r.Request.URL.Host
	domain := r.Request.Host

	_, err := db.Exec(`INSERT INTO logs (request_id, request_ip, user_agent, server_addr, domain, http_response)
		VALUES (?, ?, ?, ?, ?, ?)`,
		requestID, requestIP, userAgent, serverAddr, domain, r.StatusCode)
	if err != nil {
		return err
	}

	r.Header.Set("X-Request-ID", requestID)
	return nil
}

func reportHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Queryx(`SELECT domain, COUNT(*) AS total_requests, MAX(timestamp) AS most_recent_time
		FROM logs GROUP BY domain`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var reports []DomainReport
	for rows.Next() {
		var report DomainReport
		err = rows.StructScan(&report)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		reports = append(reports, report)
	}

	jsonData, err := json.Marshal(reports)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("ok"))
}

func generateHash(length int) string {
	chars := "76cf1d83b5e42a09"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}
