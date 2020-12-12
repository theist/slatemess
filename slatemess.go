package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"strings"
	"text/template"

	"github.com/acrazing/cheapjson"
	"github.com/joho/godotenv"
	"github.com/mitchellh/go-homedir"
	"github.com/tidwall/pretty"
	"gopkg.in/resty.v1"
)

type config struct {
	hook     string
	icon     string
	userName string
	channel  string
	message  string
	dry      bool
}

var logDebug *log.Logger

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

	ugly := pretty.Ugly([]byte(s))
	log.Printf("checking json for: %v", string(ugly))
	err := json.Unmarshal(ugly, &js)
	if err != nil {
		log.Printf("Isn't json because: %v", err)
		return false
	}
	return true
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
		msg = `{ "text": "` + messageSafe(message) + `" }`
	}
	logDebug.Printf("payload = %+v", msg)
	js, err := cheapjson.Unmarshal([]byte(msg))
	if err != nil {
		return "", err
	}
	if c.channel != "" {
		if hasKey("channel", js) {
			logDebug.Printf("WARN: channel in the payload, your specified channel %v won't be used", c.channel)
		} else {
			ch := js.AddField("channel")
			ch.AsString(c.channel)
		}
	}
	if c.userName != "" {
		if hasKey("username", js) {
			logDebug.Printf("WARN: username in the payload, your specified username %v won't be used", c.userName)
		} else {
			un := js.AddField("username")
			un.AsString(c.userName)
		}
	}
	if c.icon != "" {
		if hasKey("icon_emoji", js) {
			logDebug.Printf("WARN: icon_emoji in the payload, your specified icon %v won't be used", c.icon)
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

func toSlack(hook, payload string) error {
	res, err := resty.R().
		SetBody(payload).
		Post(hook)
	if err != nil {
		return fmt.Errorf("Error sending message %v", err)
	}
	if res.IsError() {
		return fmt.Errorf("slack api returned an error %v \"%v\"", res.Status(), string(res.Body()))
	}
	return nil
}

func toCurl(hook, payload string) {
	fmt.Println("curl -X POST -H 'Content-type: application/json' " + hook + " --data '" + strings.TrimSpace(string(pretty.Pretty([]byte(payload)))) + "'")
}

func sendMessage(c config) error {
	resty.SetDebug(false)
	message, err := messageRender(c.message)
	if err != nil {
		return err
	}
	payload, err := messageComplete(message, c)
	if err != nil {
		return err
	}
	logDebug.Printf("payload: %v", payload)
	if c.dry {
		toCurl(c.hook, payload)
	} else {
		err := toSlack(c.hook, payload)
		if err != nil {
			return err
		}
	}

	return nil
}

func readFileNameAsStr(filename string) (string, error) {
	messageFile, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("ERROR opening file %v: %v", filename, err)
	}
	messageBytes, err := ioutil.ReadAll(messageFile)
	if err != nil {
		return "", fmt.Errorf("ERROR reading file %v: %v", filename, err)
	}
	return string(messageBytes), nil
}

func main() {
	var cfg config
	logDebug = log.New(os.Stderr, "[debug] ", log.LstdFlags)
	godotenv.Load()
	homeConfigPath, err := homedir.Expand("~/.slatemess")
	if err == nil {
		godotenv.Load(homeConfigPath)
	}
	godotenv.Load("/etc/slatemess.cfg", "/etc/slack.cfg")

	fi, err := os.Stdin.Stat()
	if err != nil {
		fmt.Printf("ERROR reading stdin > %v\n", err)
		os.Exit(1)
	}
	piped := (fi.Mode() & os.ModeCharDevice) == 0

	iconArg := flag.String("icon", "", "Override default icon from hook")
	userArg := flag.String("user", "", "Override default user from hook")
	channelArg := flag.String("channel", "", "Override default user from hook")
	hookArg := flag.String("hook", "", "Override Hook provided by ENV, if any")
	messageArg := flag.String("message", "", "Provide a message by parameter")
	fileArg := flag.String("file", "", "Provide a message by file")
	debugArg := flag.Bool("debug", false, "Print debug info")
	dryArg := flag.Bool("dry", false, "Will not send the payload to slack but print a curl command equivalent, with the computed payload")
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
	if (piped && *fileArg != "") || (piped && *messageArg != "") || (*fileArg != "" && *messageArg != "") {
		fmt.Printf("ERROR: -file, -message and 'piped' operation are mutually exclusive")
		os.Exit(1)
	}
	if !*debugArg {
		logDebug.SetOutput(ioutil.Discard)
	}

	// once here only work with env or "message"
	cfg.hook = os.Getenv("SLACK_HOOK")
	cfg.icon = os.Getenv("SLACK_ICON")
	cfg.channel = os.Getenv("SLACK_CHANNEL")
	cfg.userName = os.Getenv("SLACK_USER")
	cfg.dry = *dryArg
	if piped {
		cfg.message = readStdin()
	}
	if *messageArg != "" {
		cfg.message = *messageArg
	}
	if *fileArg != "" {
		msg, err := readFileNameAsStr(*fileArg)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		cfg.message = msg

	}
	logDebug.Printf("Message: %#v", cfg)
	err = cfg.verifyConfig()
	if err != nil {
		fmt.Printf("ERROR validating parameters: %v\n", err)
		os.Exit(1)
	}
	err = sendMessage(cfg)
	if err != nil {
		fmt.Printf("ERROR Generating payload %v\n", err)
		os.Exit(1)
	}
	logDebug.Printf("Message Sent")
}
