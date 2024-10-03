package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

type Backend struct {
	URL      string `yaml:"url"`
	Up       bool
	Last     time.Time
	Host     string
	IPAddr   string
	FailTime time.Time
}

type Config struct {
	Port           int       `yaml:"port"`
	Interval       int       `yaml:"interval"`
	Health         string    `yaml:"health"`
	Backends       []Backend `yaml:"backends"`
	HealthyBackends []int
}

var (
	config     Config
	mu         sync.RWMutex
	configFile string
	dnsCache   = make(map[string]string)
	dnsMu      sync.RWMutex
)

const (
	dnsTimeout    = 5 * time.Second
	failureWindow = 30 * time.Second
)

func init() {
	flag.StringVar(&configFile, "config", "backend.yml", "Path to the backend configuration file")
	rand.Seed(time.Now().UnixNano())
}

func main() {
	flag.Parse()

	err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded configuration: %+v", config)

	go healthCheck()

	http.HandleFunc("/", proxyHandler)
	log.Printf("Starting proxy server on port %d", config.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil))
}

func loadConfig() error {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return err
	}

	if config.Port == 0 {
		config.Port = 80
	}
	if config.Interval == 0 {
		config.Interval = 3
	}
	if config.Health == "" {
		config.Health = "/health"
	}

	for i, backend := range config.Backends {
		parsedURL, err := url.Parse(backend.URL)
		if err != nil {
			log.Printf("Failed to parse backend URL %s: %v", backend.URL, err)
			continue
		}
		config.Backends[i].Host = parsedURL.Hostname()
	}

	return nil
}

func resolveHost(host string) (string, error) {
	dnsMu.RLock()
	if ip, ok := dnsCache[host]; ok {
		dnsMu.RUnlock()
		return ip, nil
	}
	dnsMu.RUnlock()

	ip, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return "", err
	}

	dnsMu.Lock()
	dnsCache[host] = ip.String()
	dnsMu.Unlock()

	return ip.String(), nil
}

func healthCheck() {
	for {
		mu.Lock()
		config.HealthyBackends = []int{}
		for i, backend := range config.Backends {
			if time.Since(backend.FailTime) < failureWindow {
				continue
			}

			ip, err := resolveHost(backend.Host)
			if err != nil {
				log.Printf("DNS lookup failed for %s: %v", backend.Host, err)
				config.Backends[i].Up = false
				config.Backends[i].FailTime = time.Now()
				continue
			}
			config.Backends[i].IPAddr = ip

			healthURL := fmt.Sprintf("http://%s%s", ip, config.Health)
			client := http.Client{Timeout: dnsTimeout}
			resp, err := client.Get(healthURL)
			if err != nil {
				log.Printf("Health check failed for %s: %v", healthURL, err)
				config.Backends[i].Up = false
				config.Backends[i].FailTime = time.Now()
			} else {
				config.Backends[i].Up = resp.StatusCode == http.StatusOK
				if config.Backends[i].Up {
					config.HealthyBackends = append(config.HealthyBackends, i)
					config.Backends[i].Last = time.Now()
				}
				resp.Body.Close()
			}
		}
		mu.Unlock()

		time.Sleep(time.Duration(config.Interval) * time.Second)
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()

	if len(config.HealthyBackends) == 0 {
		log.Printf("No healthy backends available, returning 502 Bad Gateway")
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		return
	}

	backendIndex := config.HealthyBackends[rand.Intn(len(config.HealthyBackends))]
	backend := config.Backends[backendIndex]

	director := func(req *http.Request) {
		req.URL.Scheme = "http"
		req.URL.Host = backend.IPAddr
		req.Host = backend.Host
	}

	proxy := &httputil.ReverseProxy{Director: director}
	log.Printf("Proxying request to %s (%s)", backend.Host, backend.IPAddr)
	proxy.ServeHTTP(w, r)
}