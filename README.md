# slatemess - SLAck TEmplate MESSage

This is yet another command line client for sending messages to Slack incoming web hooks.

## Why another?

I did not find any command line client that I like. Also I had a custom requirement for my client, the ability to use a template engine able to substitute environment varilables in the slack payload. So I did another client.

## Install

```text
go get -u github.com/theist/slatemess
```

## Usage

```text
   slatemess -message "<MESSAGE>" | -file <message file> [-channel <channel>] [-hook <hook url>] [-icon <slack emoji>] [-user <slack username>] [-dry] [-debug]
```

```text
Usage of slatemess:
  -channel string
        Override default user from hook
  -debug
        Print debug info
  -dry
        Will not send the payload to slack but print a curl command equivalent, with the computed payload
  -file string
        Provide a message by file
  -hook string
        Override Hook provided by ENV, if any
  -icon string
        Override default icon from hook, can be overriden by message's icon_emoji field
  -message string
        Provide a message by parameter
  -user string
        Override default user from hook
```

### Configuration precedence

Slatemess will use environment variables, but also will add environment from these files if they exists in this order:

- `.env`
- `~/.slatemess`
- `/etc/slatemess.cfg`
- `/etc/slack.cfg`

If the Env variable already exists it won't be replaced, also once a file sets a varable it won't be replaced by the subsequent files.

Environment variables, either existing or loaded from files can be overridden using parameters. Also, if the message contains fields for the icon, chanel or username, these will override both, Environment and parameters.

`slatemess` will use these environment variables

- `SLACK_HOOK`: HTTPS endpoint for the slack webhook, this can be overriden by the `-hook` parameter. Either env or `-hook` parameter is required
- `SLACK_ICON`: icon for slack message, this can be overriden by the `-icon` parameter or the field `"icon_emoji"` in the message. This is optional as every hook has an associated icon.
- `SLACK_USER`: user for slack message, this can be overriden by the `-user` parameter or the field `"username"` in the message. This is optional as every hook has an associated username.
- `SLACK_CHANNEL`: channel for sending slack message, this can be overriden by the `-channel` parameter or the field `"channel"` in the message. This is optional as every hook has an associated destination channel.

### Message mode

Message can be passed by three mutually exclusive methods to `slatemess`

- `-message` parameter. The parameter will be passed as message body
- `-file` parameter. The file will be read and passed to the message as a string
- "piped" mode: using `command | slatemess` the standard output of the message will be passed as a string to slatemess.

### Templating

The message will be interpreted as a [template using built in golang `text/template`](https://golang.org/pkg/text/template/)

Also Environment variables will be passed to the template allowing to do variable substitution with environment, for example this message template:

```go-text-template
{{ .USER }} uses {{ .EDITOR }}
```

will generate the message (with my current env):

```text
theist uses vim
```

If a env variable contains the characters '{' or '}' or '"' will not be available for substitution.

If a env variable used in substitution does not exists it will generate the string `<no value>`

The resulting messages will be passed as they are if they're detected as a valid json. If the messages aren't json but a string they will be enclosed in a basic slack message payload with this shape:

```json
{
    "text": "the message json safe"
}
```

Using this feature is possible to send rich format messages to slack using [slack `blocks` syntax](https://api.slack.com/reference/block-kit/blocks) there's a sample under `samples/blocks` that will generate a message similar to this

![blocks message](https://github.com/theist/slatemess/blob/media/sample_message.png?raw=true)

with the correct environment and a commandline like this:

```shell
slatemess -file samples/blocks -user slatemess -icon :ok_hand: -hook <hook_url>
```

**WARNING**: If a template contains a valid field `icon_emoji`, `channel` or `username` these won't be overwritten and will override any value passed by environment or variables

**WARNING**: Once a message is detected as json it will be sent as is, but completed with `icon_emoji`, `channel` and `username` if aren't already present. That won't restrain you from sending an invalid message to slack that won't produce any message.

### Output as Curl

If parameter flag `-dry` is used it will show a curl command with the appropiate data payload and parameters instead of posting it to slack.
