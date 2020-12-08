package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"text/template"

	"github.com/acrazing/cheapjson"
	"github.com/joho/godotenv"
	"github.com/mitchellh/go-homedir"
	"gopkg.in/resty.v1"
)

type config struct {
	hook     string
	icon     string
	userName string
	channel  string
	message  string
}

func readStdin() string {
	scanner := bufio.NewScanner(os.Stdin)
	text := ""
	for scanner.Scan() {
		text += scanner.Text()
		text += "\n"
	}
	return text
}

func (c config) verifyConfig() error {
	if c.message == "" {
		return fmt.Errorf("Missing Message")
	}
	u, err := url.Parse(c.hook)
	if err != nil {
		return fmt.Errorf("Error in url %v: %v", c.hook, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("Invalid Hook, invalid scheme %v", u.Scheme)
	}
	return nil
}

func isJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}

// checks string for problematic chars
func forbiddenVal(s string) bool {
	forbiddenChars := []string{"{", "}", "\"", "\\"}
	for _, char := range forbiddenChars {
		if strings.Contains(s, char) {
			return true
		}
	}
	return false
}

// Returns Env as map key value
func dictEnviron() map[string]string {
	dict := make(map[string]string)
	for _, env := range os.Environ() {
		split := strings.SplitN(env, "=", 2)
		if forbiddenVal(split[1]) {
			continue
		}
		dict[split[0]] = split[1]
	}
	return dict
}

// Renders a message doing env sustitution
func messageRender(message string) (string, error) {
	var render bytes.Buffer
	t, err := template.New("message").Parse(message)
	if err != nil {
		return "", fmt.Errorf("Error rendering slack template: %v", err)
	}
	err = t.Execute(&render, dictEnviron())
	if err != nil {
		return "", fmt.Errorf("Error generating slack payload: %v", err)
	}
	return render.String(), nil
}

func messageSafe(message string) string {
	safe := strings.TrimSpace(message)
	safe = strings.ReplaceAll(safe, "\n", `\n`)
	safe = strings.ReplaceAll(safe, `"`, `\"`)
	return safe
}

func hasKey(key string, j *cheapjson.Value) bool {
	k := j.Get(key)
	return k != nil
}

func messageComplete(message string, c config) (string, error) {
	msg := ""
	if isJSON(message) {
		msg = message
	} else {
		msg = `{ "text": "` + message + `" }`
	}
	log.Printf("payload = %+v", msg)
	js, err := cheapjson.Unmarshal([]byte(msg))
	if err != nil {
		return "", err
	}
	if c.channel != "" {
		if hasKey("channel", js) {
			log.Printf("WARN: channel in the payload, your specified channel %v won't be used", c.channel)
		} else {
			ch := js.AddField("channel")
			ch.AsString(c.channel)
		}
	}
	if c.userName != "" {
		if hasKey("username", js) {
			log.Printf("WARN: username in the payload, your specified username %v won't be used", c.userName)
		} else {
			un := js.AddField("username")
			un.AsString(c.userName)
		}
	}
	if c.icon != "" {
		if hasKey("icon_emoji", js) {
			log.Printf("WARN: icon_emoji in the payload, your specified icon %v won't be used", c.icon)
		} else {
			ie := js.AddField("icon_emoji")
			ie.AsString(c.icon)
		}
	}

	data := js.Value()
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func sendMessageToSlack(c config) error {
	resty.SetDebug(false)
	message, err := messageRender(c.message)
	if err != nil {
		return err
	}
	payload, err := messageComplete(messageSafe(message), c)
	if err != nil {
		return err
	}
	log.Printf("payload: %v", payload)

	res, err := resty.R().
		SetBody(payload).
		Post(c.hook)
	if err != nil {
		return fmt.Errorf("Error sending message %v", err)
	}
	if res.IsError() {
		return fmt.Errorf("slack api returned an error %v \"%v\"", res.Status(), string(res.Body()))
	}
	return nil
}

func main() {
	var cfg config
	log.Print("slatemess started")
	godotenv.Load()
	homeConfigPath, err := homedir.Expand("~/.slatemess")
	if err == nil {
		godotenv.Load(homeConfigPath)
	}
	godotenv.Load("/etc/slatemess.cfg", "/etc/slack.cfg")

	fi, err := os.Stdin.Stat()
	if err != nil {
		log.Fatal("Unable to read stdin >", err)
	}
	piped := (fi.Mode() & os.ModeCharDevice) == 0

	iconArg := flag.String("icon", "", "Override icon")
	userArg := flag.String("user", "", "Override user")
	hookArg := flag.String("hook", "", "Override Hook URL")
	channelArg := flag.String("channel", "", "Override channel")

	flag.Parse()

	if *iconArg != "" {
		os.Setenv("SLACK_ICON", *iconArg)
	}
	if *userArg != "" {
		os.Setenv("SLACK_USER", *userArg)
	}
	if *hookArg != "" {
		os.Setenv("SLACK_HOOK", *hookArg)
	}
	if *channelArg != "" {
		os.Setenv("SLACK_CHANNEL", *channelArg)
	}

	// once here only work with env or "message"
	cfg.hook = os.Getenv("SLACK_HOOK")
	cfg.icon = os.Getenv("SLACK_ICON")
	cfg.channel = os.Getenv("SLACK_CHANNEL")
	cfg.userName = os.Getenv("SLACK_USER")
	if piped {
		cfg.message = readStdin()
	}
	log.Printf("Message: %#v", cfg)
	err = cfg.verifyConfig()
	if err != nil {
		log.Printf("Error validating parameters: %v", err)
		os.Exit(1)
	}
	err = sendMessageToSlack(cfg)
	if err != nil {
		log.Printf("Error Generating payload %v", err)
	}
}
