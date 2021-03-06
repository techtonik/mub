package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/chzyer/readline"
)

type command struct {
	Name      string
	Prototype interface{}
	Desc      string
}

type nickname string
type channel string
type nickorchan string

type nocommand struct{}

type awaycommand struct {
}

type helpcommand struct{}

type tlsconnectcommand struct {
	Server string "IRC server"
	Nick   string "Your nickname"
}

type connectcommand struct {
	Server string "IRC server"
	Nick   string "Your nickname"
}

type quitcommand struct{}

type querycommand struct {
	Target nickorchan "channel or nick"
}

type joincommand struct {
	Channel channel "channel"
}

type whoiscommand struct {
	Nick nickname "nick"
}

type nickcommand struct {
	Nick nickname "nick"
}

type partcommand struct {
	Channel channel "channel"
}

type mecommand struct {
	Action string
}

type msgcommand struct {
	Nick nickname "nick"
}

type namescommand struct{}

type statuscommand struct{}

// commandState is the internal state of completer.
type commandState struct {
	FoundCmd int
	Channels map[string]string
	NickMap  map[string]string
}

// commands keeps a list of commands and the internal state of the
// completer.
type Commands struct {
	Commands []command
	State    *commandState
}

// Constants for output type
const (
	Ostdio int = iota
	Oreadline
)

// Output is the type of output and associated writer.
type Output struct {
	Type   int // From constants above.
	Output io.Writer
}

var (
	output Output // All output should go here, not to stdout.

	statusEvents bool = true // Show joined, parted, quit, ... messages

	commands = Commands{
		Commands: []command{
			{"", nocommand{}, "No command given."},
			{"/away", awaycommand{}, "Toggle presence."},
			{"/help", helpcommand{}, "Give this help"},
			{"/tlsconnect", tlsconnectcommand{}, "Connect to IRC server using TLS."},
			{"/connect", connectcommand{}, "Connect to IRC server."},
			{"/quit", quitcommand{}, "Quit the IRC client."},
			{"/query", querycommand{}, "Start talking to a nick or channel."},
			{"/x", querycommand{}, "Shorthand for /query."},
			{"/join", joincommand{}, "Join a channel."},
			{"/part", partcommand{}, "Leave a channel."},
			{"/whois", whoiscommand{}, "Show information about someone."},
			{"/me", mecommand{}, "Show a string describing you doing something."},
			{"/msg", msgcommand{}, "Send a message to a specific target."},
			{"/nick", nickcommand{}, "Change your nickname."},
			{"/names", namescommand{}, "List members on current channel."},
			{"/status", statuscommand{}, "Toggle status join, quit messages."}},
	}
)

// CompleteNickOrChan completes either a channel name if our current
// position begins with an "#" or, if not, a nickname of a user.
func CompleteNickOrChan(linestr string, space int, wordpos int, channels map[string]string, nicks map[string]string) (newLine [][]rune) {
	if strings.HasPrefix(linestr[space+1:], "#") {
		// Complete a channel.
		newLine = findmap(linestr[space+1:], channels, wordpos, "")
	} else {
		// Complete a nickname.
		newLine = findmap(linestr[space+1:], nicks, wordpos, "")
	}

	return
}

// Do is a completer for readline.
func (c Commands) Do(line []rune, pos int) (newLine [][]rune, length int) {
	var linestr = string(line)
	var matches int

	// Find where the first space is in command string.
	space := strings.IndexRune(linestr, ' ')
	if space == -1 {
		// There is no space. We're writing the first word.
		if len(line) != 0 && linestr[0] == '/' {
			// This is a command completion.
			for i, cmd := range c.Commands {
				if strings.HasPrefix(cmd.Name, strings.ToLower(linestr)) {
					newLine = append(newLine, []rune(cmd.Name[pos:]+" "))
					matches++
					c.State.FoundCmd = i
				}
			}
			if matches != 1 {
				c.State.FoundCmd = 0
			}
		} else {
			// Nick completion.
			newLine = findmap(linestr[space+1:], c.State.NickMap, pos, ": ")
		}
	} else {
		// Argument completion.

		// The line so far is...
		head := linestr[:space] + " "
		// ...and our position in the this word is:
		wordpos := pos - len(head)

		switch c.Commands[c.State.FoundCmd].Prototype.(type) {
		case msgcommand:
			newLine = CompleteNickOrChan(linestr, space, wordpos,
				c.State.Channels, c.State.NickMap)
		case querycommand:
			newLine = CompleteNickOrChan(linestr, space, wordpos,
				c.State.Channels, c.State.NickMap)
		case whoiscommand:
			newLine = findmap(linestr[space+1:], c.State.NickMap, wordpos, "")
		case joincommand:
			newLine = findmap(linestr[space+1:], c.State.Channels, wordpos, "")
		case partcommand:
			newLine = findmap(linestr[space+1:], c.State.Channels, wordpos, "")
		}
	}

	length = len(linestr)

	return
}

func findmatch(arg string, args []string, wordpos int) (newLine [][]rune) {
	arg = strings.ToLower(arg)
	for _, n := range args {
		if strings.HasPrefix(strings.ToLower(n), arg) {
			newLine = append(newLine, []rune(n[wordpos:]))
		}
	}

	return
}

// Look for argument with prefix arg in the map args. wordpos is where
// our cursor is. Just return whatever is after that.
//
// Returns an array of completion lines.
func findmap(arg string, args map[string]string, wordpos int, suffix string) (newLine [][]rune) {
	arg = strings.ToLower(arg)
	for _, n := range args {
		if strings.HasPrefix(strings.ToLower(n), arg) {
			newLine = append(newLine, []rune(n[wordpos:]+suffix))
		}
	}

	return
}

func errormsg(msg string) {
	message(msg)
}

func info(msg string) {
	message(msg)
}

func warn(msg string) {
	message(msg)
}

// An incoming message or action from another participant.
func showmsg(nick string, target string, text string, action bool) {
	var str string

	if action {
		str = fmt.Sprintf("%v [%v %v]", target, nick, text)
	} else {
		str = fmt.Sprintf("%v <%v> %v", target, nick, text)
	}

	message(str)
}

// Sanitize string msg from ESC and control characters.
//
// Returns sanitized string in out.
func sanitizestring(msg string) (out string) {
	for _, c := range msg {
		if c == 127 || (c < 32 && c != '\t') {
			out = out + "?"
		} else {
			out = out + string(c)
		}
	}

	return out
}

// Wrap words in msg at column col. When wrapping, prefix each new
// line with nine spaces to fit under timestamp.
//
// Returns new wrapped message in out.
func wrap(msg string, col int) (out string) {
	fields := strings.Fields(msg)

	var linelen int
	for _, field := range fields {
		linelen = linelen + len(field) + 1
		if linelen < col {
			// Still the same line.
			out = out + field + " "
		} else {
			// This is a new line.
			linelen = len(field) + 7
			out = out + "\n       " + field + " "
		}
	}

	return
}

// All messages to user should be sent through this function, which
// sanitizes them, timestamps them and possibly word wraps and might
// do other things depending on output type.
func message(msg string) {
	timestr := time.Now().Format("15:04")
	msg = sanitizestring(msg)
	msg = fmt.Sprintf("%v %s", timestr, msg)

	msg = wrap(msg, 72)

	fmt.Fprintf(output.Output, "%s\n", msg)
}

func printhelp() {
	for _, cmd := range commands.Commands {
		msg := cmd.Name
		prototype := reflect.TypeOf(cmd.Prototype)
		for i := 0; i < prototype.NumField(); i++ {
			msg += " <" + strings.ToLower(prototype.Field(i).Name) + ">"
		}

		message(msg + " - " + cmd.Desc)
	}
}

func parsecommand(line string) {
	fields := strings.Fields(line)
	// Calculate line pos of where first & second argument begins -- for
	// using as "rest of line" by relevant commands. Does not omit any
	// initial spaces of those arguments, ie:
	// line:/me  slaps quite
	//          ^- firstpos
	// line:/msg   quite   . . . it was a trout
	//                   ^- secondpos
	firstpos := 0
	secondpos := 0
	if len(fields) >= 2 {
		firstpos = strings.Index(line, " ")
		firstpos++
	}
	if len(fields) >= 3 {
		secondpos = firstpos
		// skipping all spaces between command and first arg
		for line[secondpos] == ' ' {
			secondpos++
		}
		secondpos += strings.Index(line[secondpos:], " ")
		secondpos++
	}

	// Check if this command is allowed.
	if _, val := conf.BlockedCommands[fields[0]]; val {
		info("Command blocked by configuration.")
		return
	}

	switch fields[0] {
	case "/away":
		if conn == nil {
			noconnection()
			break
		}
		if len(fields) >= 2 {
			conn.Away(line[firstpos:])
			away()
		} else {
			conn.Away()
			back()
		}

	case "/help":
		printhelp()

	case "/tlsconnect":
		var pass string

		if len(fields) < 3 {
			warn("Use /connect server:port nick [server-pass]")
			return
		}
		if len(fields) == 4 {
			pass = fields[3]
		}

		connect(fields[1], fields[2], pass, true)

	case "/connect":
		var pass string

		if len(fields) < 3 {
			warn("Use /connect server:port nick [server-pass]")
			return
		}
		if len(fields) == 4 {
			pass = fields[3]
		}

		connect(fields[1], fields[2], pass, false)

	case "/nick":
		if conn == nil {
			noconnection()
			break
		}

		conn.Nick(fields[1])

	case "/join":
		if conn == nil {
			noconnection()
			break
		}

		if len(fields) != 2 {
			warn("Use /join #channel")
			return
		}

		currtarget = fields[1]
		conn.Join(currtarget)
		commands.State.Channels[currtarget] = currtarget
	case "/part":
		if conn == nil {
			noconnection()
			break
		}

		if len(fields) != 2 {
			warn("Use /part #channel")
			return
		}

		conn.Part(fields[1])
		currtarget = ""
		// Forget about this channel
		delete(commands.State.Channels, currtarget)
	case "/me":
		if conn == nil {
			noconnection()
			break
		}

		if len(fields) < 2 {
			warn("Use /me action text")
			return
		}

		conn.Action(currtarget, line[firstpos:])
		logmsg(time.Now(), conn.Me().Nick, currtarget, line[firstpos:], true)

	case "/names":
		if conn == nil {
			noconnection()
			break
		}

		namescmd := fmt.Sprintf("NAMES %v", currtarget)
		conn.Raw(namescmd)

	case "/status":
		if statusEvents {
			statusEvents = false
			message("Not showing quits, joins, et cetera.")
		} else {
			statusEvents = true
			message("Showing quits, joins, et cetera.")
		}

	case "/whois":
		if conn == nil {
			noconnection()
			break
		}

		if len(fields) != 2 {
			warn("Use /whois <nick>")
			return
		}

		conn.Whois(fields[1])

	case "/msg":
		if conn == nil {
			noconnection()
			break
		}

		if len(fields) < 3 {
			warn("Use /msg target message text")
			return
		}

		conn.Privmsg(fields[1], line[secondpos:])
		logmsg(time.Now(), conn.Me().Nick, fields[1], line[secondpos:], false)
	case "/x":
		fallthrough
	case "/query":
		if conn == nil {
			noconnection()
			break
		}

		if len(fields) != 2 {
			warn("Use /query <nick/channel>")
			return
		}

		currtarget = fields[1]

	case "/quit":
		iquit()
		if conn != nil {
			if len(fields) == 2 {
				conn.Quit(fields[1])
			} else {
				conn.Quit()
			}
		}

		quitclient = true

	default:
		warn("Unknown command: " + fields[0])
	}
}

func initUI(subprocess bool) (rl *readline.Instance, bio *bufio.Reader) {
	var err error

	commands.State = new(commandState)
	commands.State.NickMap = make(map[string]string)
	commands.State.Channels = make(map[string]string)

	if subprocess {
		// We're running as a subprocess. Just read from stdin.
		bio = bufio.NewReader(os.Stdin)
		output.Type = Ostdio
		output.Output = os.Stdout
	} else {
		// Internal state for command completer.

		// Slightly smarter UI is used.
		rl, err = readline.NewEx(&readline.Config{
			AutoComplete: commands,
		})
		if err != nil {
			panic(err)
		}

		// Send output to readline's handler so prompt can
		// refresh.
		output.Output = rl.Stdout()
		output.Type = Oreadline
	}

	return
}

func ui(subprocess bool, rl *readline.Instance, bio *bufio.Reader) {

	var line string
	var err error

	if !subprocess {
		defer rl.Close()
	}

	quitclient = false
	for !quitclient {
		if subprocess {
			line, err = bio.ReadString('\n')
			if err != nil {
				log.Fatal("Couldn't get input.\n")
			}
		} else {
			rl.SetPrompt("\033[33m" + currtarget + "> \033[0m")
			line, err = rl.Readline()
			if err != nil {
				break
			}
		}

		if line != "" && line != "\n" && line != "\r\n" {
			if line[0] == '/' {
				// A command
				parsecommand(line)
			} else {
				// Send line to target.
				if currtarget == "" {
					notarget()
				} else {
					conn.Privmsg(currtarget, line)
					logmsg(time.Now(), conn.Me().Nick, currtarget, line, false)
				}
			}
		}
	}
}
