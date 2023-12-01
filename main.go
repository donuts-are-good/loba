package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net"
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

var (
	nodeHealthCheckCycleTime = 60 * time.Second
	timeout                  = time.Second * 2
	BuildNumber              = "4"
	db                       *sqlx.DB
	servers                  map[string][]Server
	current                  int
	configMutex              = &sync.Mutex{}
	initLogsTable            = `CREATE TABLE IF NOT EXISTS logs ( request_id TEXT, request_ip TEXT, user_agent TEXT, server_addr TEXT, domain TEXT, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP, http_response INTEGER);`
	insertLogEntry           = `INSERT INTO logs (request_id, request_ip, user_agent, server_addr, domain, http_response) VALUES (?, ?, ?, ?, ?, ?)`
	enumerateDomainStats     = `SELECT domain, COUNT(*) AS total_requests, MAX(timestamp) AS most_recent_time
			FROM logs GROUP BY domain`
)

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

func main() {
	var err error
	nodeHealthCheckCycle()
	initSqlite(err)
	loadConfig()
	routeHandlers()
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func routeHandlers() {
	http.HandleFunc("/", proxyHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/report", reportHandler)
	http.HandleFunc("/reload", reloadHandler)
}

func initSqlite(err error) {
	db, err = sqlx.Connect("sqlite3", "./loadbalancer.db")
	if err != nil {
		log.Fatalln(err)
	}

	_, err = db.Exec(initLogsTable)
	if err != nil {
		log.Fatalln(err)
	}
}

func loadConfig() map[string][]Server {
	data, err := os.ReadFile("./config/domains.json")
	if err != nil {
		log.Println("Error reading domains.json:", err)
		return nil
	}

	var config []DomainConfig
	err = json.Unmarshal(data, &config)
	if err != nil {
		log.Println("Error parsing domains.json:", err)
		return nil
	}

	newServers := make(map[string][]Server)
	for _, domainConfig := range config {
		for _, serverAddr := range domainConfig.Servers {
			if checkServerHealth(serverAddr) {
				newServers[domainConfig.Domain] = append(newServers[domainConfig.Domain], Server{
					Addr:   serverAddr,
					Domain: domainConfig.Domain,
					Active: true,
				})
			}
		}
	}

	return newServers
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.Lock()
	defer configMutex.Unlock()

	domainServers, ok := servers[r.Host]
	if !ok || len(domainServers) == 0 {
		http.Error(w, "No servers available for domain", http.StatusInternalServerError)
		return
	}

	var server Server
	for i := 0; i < len(domainServers); i++ {
		server = domainServers[current]
		current = (current + 1) % len(domainServers)
		if server.Active {
			break
		}
	}

	if !server.Active {
		http.Error(w, "No active servers available for domain", http.StatusInternalServerError)
		return
	}

	target, err := url.Parse(server.Addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = logResponse
	proxy.ServeHTTP(w, r)
}

func nodeHealthCheckCycle() {
	go func() {
		for range time.Tick(nodeHealthCheckCycleTime) {
			checkAllServerHealth()
		}
	}()
}

func logResponse(r *http.Response) error {
	requestID := generateHash(32)
	requestIP := strings.Split(r.Request.RemoteAddr, ":")[0]
	userAgent := r.Request.UserAgent()
	serverAddr := r.Request.URL.Host
	domain := r.Request.Host

	_, err := db.Exec(insertLogEntry,
		requestID, requestIP, userAgent, serverAddr, domain, r.StatusCode)
	if err != nil {
		return err
	}

	r.Header.Set("X-Request-ID", requestID)
	return nil
}

func reportHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Queryx(enumerateDomainStats)
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

func checkServerHealth(serverAddr string) bool {

	conn, err := net.DialTimeout("tcp", serverAddr+"/v0/health", timeout)
	if err != nil {
		log.Println("Server health check failed:", err)
		return false
	}
	conn.Close()
	return true
}

func checkAllServerHealth() {
	configMutex.Lock()
	defer configMutex.Unlock()

	for domain, domainServers := range servers {
		for i, server := range domainServers {
			if !checkServerHealth(server.Addr) {
				servers[domain] = append(domainServers[:i], domainServers[i+1:]...)
				log.Println("Server removed:", server.Addr)
			}
		}
	}
}

func reloadHandler(w http.ResponseWriter, r *http.Request) {
	newServers := loadConfig()
	if newServers == nil {
		http.Error(w, "Invalid configuration", http.StatusInternalServerError)
		return
	}

	configMutex.Lock()
	servers = newServers
	configMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("Configuration reloaded"))
}
