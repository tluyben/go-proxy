package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

type Backend struct {
	URL string `yaml:"url"`
	Up  bool
}

type Config struct {
	Port     int       `yaml:"port"`
	Interval int       `yaml:"interval"`
	Health   string    `yaml:"health"`
	Backends []Backend `yaml:"backends"`
}

var (
	config     Config
	mu         sync.RWMutex
	configFile string
)

func init() {
	flag.StringVar(&configFile, "config", "backend.yml", "Path to the backend configuration file")
}

func main() {
	flag.Parse()

	err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

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

	// Set default values if not specified in the config file
	if config.Port == 0 {
		config.Port = 80
	}
	if config.Interval == 0 {
		config.Interval = 3
	}
	if config.Health == "" {
		config.Health = "/health"
	}

	return nil
}

func healthCheck() {
	for {
		mu.Lock()
		for i, backend := range config.Backends {
			backendURL, err := url.Parse(backend.URL)
			if err != nil {
				config.Backends[i].Up = false
				continue
			}

			_, err = net.LookupHost(backendURL.Hostname())
			if err != nil {
				config.Backends[i].Up = false
				continue
			}

			resp, err := http.Get(backend.URL + config.Health)
			if err != nil || resp.StatusCode != http.StatusOK {
				config.Backends[i].Up = false
			} else {
				config.Backends[i].Up = true
			}
		}
		mu.Unlock()

		time.Sleep(time.Duration(config.Interval) * time.Second)
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	defer mu.RUnlock()

	for _, backend := range config.Backends {
		if !backend.Up {
			continue
		}

		backendURL, err := url.Parse(backend.URL)
		if err != nil {
			continue
		}

		// Perform an additional health check
		resp, err := http.Get(backend.URL + config.Health)
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		// Forward the request to the backend
		proxy := httputil.NewSingleHostReverseProxy(backendURL)
		proxy.ServeHTTP(w, r)
		return
	}

	// If all backends are down, return a Gateway error
	http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
}
