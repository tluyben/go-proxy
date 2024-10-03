package main

import (
	"context"
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
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v2"
)

type Backend struct {
	URL    string `yaml:"url"`
	Host   string
	Port   string
	Health int32 // 0 for down, 1 for up
}

type Config struct {
	Port     int       `yaml:"port"`
	Interval int       `yaml:"interval"`
	Health   string    `yaml:"health"`
	Backends []Backend `yaml:"backends"`
}

var (
	config     Config
	configFile string
	dnsCache   = make(map[string]string)
	dnsMu      sync.RWMutex
	verbose    bool
)

const (
	dnsTimeout    = 500 * time.Millisecond
	httpTimeout   = 5 * time.Second
)

func init() {
	flag.StringVar(&configFile, "config", "backend.yml", "Path to the backend configuration file")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	rand.Seed(time.Now().UnixNano())
}

func main() {
	flag.Parse()

	// Set up logging
	if !verbose {
		log.SetOutput(ioutil.Discard)
	}

	err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if verbose {
		log.Printf("Loaded configuration: %+v", config)
	}

	go healthCheck()

	http.HandleFunc("/", proxyHandler)
	log.Printf("Starting proxy server on port %d", config.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil))
}

func loadConfig() error {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %v", err)
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
		config.Backends[i].Port = parsedURL.Port()
		if config.Backends[i].Port == "" {
			config.Backends[i].Port = "80"
		}
	}

	return nil
}

func resolveHostWithTimeout(host string) (string, error) {
	dnsMu.RLock()
	if ip, ok := dnsCache[host]; ok {
		dnsMu.RUnlock()
		return ip, nil
	}
	dnsMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), dnsTimeout)
	defer cancel()

	resolver := &net.Resolver{}
	ips, err := resolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %v", host, err)
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no IPs found for %s", host)
	}

	ip := ips[0].String()

	dnsMu.Lock()
	dnsCache[host] = ip
	dnsMu.Unlock()

	return ip, nil
}

func healthCheck() {
	for {
		for i, backend := range config.Backends {
			go func(i int, backend Backend) {
				healthURL := fmt.Sprintf("%s%s", backend.URL, config.Health)
				client := http.Client{Timeout: httpTimeout}
				resp, err := client.Get(healthURL)
				if err != nil {
					if verbose {
						log.Printf("Health check failed for %s: %v", healthURL, err)
					}
					atomic.StoreInt32(&config.Backends[i].Health, 0)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					atomic.StoreInt32(&config.Backends[i].Health, 1)
					if verbose {
						log.Printf("Backend %s is healthy", backend.URL)
					}
				} else {
					atomic.StoreInt32(&config.Backends[i].Health, 0)
					if verbose {
						log.Printf("Backend %s is unhealthy, status code: %d", backend.URL, resp.StatusCode)
					}
				}
			}(i, backend)
		}
		time.Sleep(time.Duration(config.Interval) * time.Second)
	}
}

func getHealthyBackend() (*Backend, error) {
	healthyBackends := make([]*Backend, 0)
	for i := range config.Backends {
		if atomic.LoadInt32(&config.Backends[i].Health) == 1 {
			healthyBackends = append(healthyBackends, &config.Backends[i])
		}
	}

	if len(healthyBackends) == 0 {
		return nil, fmt.Errorf("no healthy backends available")
	}

	return healthyBackends[rand.Intn(len(healthyBackends))], nil
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	if verbose {
		log.Printf("Received request for %s", r.URL.Path)
	}

	backend, err := getHealthyBackend()
	if err != nil {
		if verbose {
			log.Printf("No healthy backends available: %v", err)
		}
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		if verbose {
			log.Printf("Failed to parse backend URL: %v", err)
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(backendURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if verbose {
			log.Printf("Proxy error: %v", err)
		}
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.Transport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if verbose {
		log.Printf("Proxying request to %s", backend.URL)
	}
	proxy.ServeHTTP(w, r)

	if verbose {
		log.Printf("Request completed in %v", time.Since(startTime))
	}
}