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
	client paho.Client
	topic  string

	mu          sync.RWMutex
	handler     SubHandler
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

	hostname := "unknown"
	if h, err := os.Hostname(); err == nil {
		hostname = h
	}

	c := &Client{topic: topic}

	opts := paho.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s", host))
	opts.SetClientID(fmt.Sprintf("mqvision_%s_%d", hostname, os.Getpid()))
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetKeepAlive(60 * time.Second)
	opts.OnConnect = c.onConnect
	opts.OnConnectionLost = c.onConnectionLost

	c.client = paho.NewClient(opts)
	return c, nil
}

// Run registers the subscription handler and starts connecting.
// Subscriptions are (re)established in OnConnect so AutoReconnect restores them.
func (c *Client) Run(h SubHandler) error {
	c.mu.Lock()
	c.handler = h
	c.mu.Unlock()

	token := c.client.Connect()
	go func() {
		token.Wait()
		if err := token.Error(); err != nil {
			log.Printf("MQTT connect ended with error: %v", err)
			c.mu.Lock()
			c.lastError = err
			c.mu.Unlock()
		}
	}()

	return nil
}

func (c *Client) onConnect(client paho.Client) {
	c.mu.Lock()
	c.isConnected = true
	c.lastError = nil
	handler := c.handler
	topic := c.topic
	c.mu.Unlock()

	log.Println("MQTT connected")

	if handler == nil {
		return
	}

	if token := client.Subscribe(topic, 0, newMessageHandler(handler, c)); token.Wait() && token.Error() != nil {
		err := fmt.Errorf("error subscribing to topic %s: %w", topic, token.Error())
		log.Print(err)
		c.mu.Lock()
		c.lastError = err
		c.mu.Unlock()
		return
	}
	log.Printf("MQTT subscribed to %s", topic)
}

func (c *Client) onConnectionLost(_ paho.Client, err error) {
	c.mu.Lock()
	c.isConnected = false
	c.lastError = err
	c.mu.Unlock()
	log.Printf("MQTT connection lost: %v", err)
}

// Status returns the current connection status and the last error encountered.
func (c *Client) Status() (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isConnected, c.lastError
}

func (c *Client) Stop() error {
	c.mu.RLock()
	topic := c.topic
	connected := c.isConnected
	c.mu.RUnlock()

	if connected {
		if token := c.client.Unsubscribe(topic); token.Wait() && token.Error() != nil {
			return fmt.Errorf("error unsubscribing from topic: %v", token.Error())
		}
	}
	c.client.Disconnect(1000)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.RLock()
		connected = c.isConnected
		c.mu.RUnlock()
		if !connected {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func newMessageHandler(h SubHandler, c *Client) paho.MessageHandler {
	return func(client paho.Client, msg paho.Message) {
		wc := h()
		if wc == nil {
			err := fmt.Errorf("error getting writer")
			c.mu.Lock()
			c.lastError = err
			c.mu.Unlock()
			return
		}
		defer wc.Close()

		_, err := wc.Write(msg.Payload())
		if err != nil {
			c.mu.Lock()
			c.lastError = fmt.Errorf("error writing to writer: %v", err)
			c.mu.Unlock()
			return
		}

		c.mu.Lock()
		c.lastError = nil
		c.mu.Unlock()
	}
}
