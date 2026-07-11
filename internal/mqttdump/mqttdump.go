package mqttdump

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type SubHandler func() io.WriteCloser

type Client struct {
	client      paho.Client
	topic       string
	chError     chan error
	chConnected chan bool

	mu          sync.RWMutex
	isConnected bool
	lastError   error
}

func NewClient(addr string, topic string) (*Client, error) {
	uri, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("error parsing MQTT URI: %v", err)
	}

	host := uri.Host
	if uri.Port() == "" {
		host += ":1883" // Default MQTT port
	}

	username := uri.User.Username()
	password, _ := uri.User.Password()

	chErr := make(chan error, 1)
	chConnected := make(chan bool, 1)
	chConnected <- false

	hostname := "unknown"
	if h, err := os.Hostname(); err == nil {
		hostname = h
	}
	opts := paho.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s", host))
	opts.SetClientID(fmt.Sprintf("mqvision_%s_%d", hostname, os.Getpid()))
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.OnConnect = newConnectHandler(chConnected, chErr)
	opts.OnConnectionLost = newConnectionLostHandler(chConnected, chErr)
	opts.SetAutoReconnect(true)
	opts.SetKeepAlive(60 * time.Second)

	return &Client{
		client:      paho.NewClient(opts),
		topic:       topic,
		chError:     chErr,
		chConnected: chConnected,
	}, nil
}

func (c *Client) Run(h SubHandler) error {
	go func() {
		for err := range c.chError {
			if err != nil {
				log.Printf("Error in MQTT client: %v", err)
			}
			c.mu.Lock()
			c.lastError = err
			c.mu.Unlock()
		}
	}()

	go func() {
		for isConnected := range c.chConnected {
			c.mu.Lock()
			c.isConnected = isConnected
			c.mu.Unlock()
		}
	}()

	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("error connecting to MQTT broker: %v", token.Error())
	}

	// Subscribe to topic with message handler
	if token := c.client.Subscribe(c.topic, 0, newMessageHandler(h, c.chError)); token.Wait() && token.Error() != nil {
		return fmt.Errorf("error subscribing to topic: %v", token.Error())
	}

	return nil
}

// Status returns the current connection status and the last error encountered.
func (c *Client) Status() (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isConnected, c.lastError
}

func (c *Client) Stop() error {
	if token := c.client.Unsubscribe(c.topic); token.Wait() && token.Error() != nil {
		return fmt.Errorf("error unsubscribing from topic: %v", token.Error())
	}
	c.client.Disconnect(1000)

	tkr := time.NewTicker(time.Second)
	defer tkr.Stop()

	for range tkr.C {
		c.mu.RLock()
		connected := c.isConnected
		c.mu.RUnlock()
		if !connected {
			break
		}
	}

	close(c.chError)
	close(c.chConnected)

	return nil
}

func newConnectHandler(chConnected chan bool, chError chan error) paho.OnConnectHandler {
	return func(client paho.Client) {
		chConnected <- true
		chError <- nil
	}
}

func newConnectionLostHandler(chConnected chan bool, chError chan error) paho.ConnectionLostHandler {
	return func(client paho.Client, err error) {
		chConnected <- false
		chError <- err
	}
}

func newMessageHandler(h SubHandler, chError chan error) paho.MessageHandler {
	return func(client paho.Client, msg paho.Message) {
		// timestamp := time.Now().Format("20060102_150405")
		// filename := fmt.Sprintf("gauge_%s.jpg", timestamp)

		// // Create output directory if it doesn't exist
		// outputDir := "images"
		// if err := os.MkdirAll(outputDir, 0755); err != nil {
		// 	log.Printf("Error creating directory: %v", err)
		// 	chError <- fmt.Errorf("error creating directory: %v", err)
		// 	return
		// }

		// filepath := filepath.Join(outputDir, filename)

		// Write JPEG bytes to file
		// if err := os.WriteFile(filepath, msg.Payload(), 0644); err != nil {
		// 	log.Printf("Error writing file %s: %v", filepath, err)
		// 	chError <- fmt.Errorf("error writing file %s: %v", filepath, err)
		// 	return
		// }

		wc := h()
		if wc == nil {
			chError <- fmt.Errorf("error getting writer")
			return
		}
		defer wc.Close()

		_, err := wc.Write(msg.Payload())
		if err != nil {
			chError <- fmt.Errorf("error writing to writer: %v", err)
			return
		}

		chError <- nil
	}
}
