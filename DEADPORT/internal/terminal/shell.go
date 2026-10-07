package terminal

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Output limits so no command can produce an absurd amount of text.
const (
	MaxOutputLines = 1500
	MaxHistory     = 100
)

// Action tells the UI layer that a command needs game-level handling.
type Action int

// Actions returned in Result.
const (
	ActNone Action = iota
	ActClear
	ActExit
	ActSubmit
	ActHint
	ActObjective
	ActChat
	ActFix
)

// Result is the outcome of executing one command line.
type Result struct {
	Output string
	Action Action
	Arg    string
	// Secret is set when the player found an easter egg command.
	Secret string
	// Error is true when Output describes a failure (for styling).
	Error bool
}

// Options configures a Shell.
type Options struct {
	Home  string   // starting directory
	User  string   // shown by whoami / in the prompt
	Host  string   // shown in the prompt
	Tools []string // optional tools enabled for this mission (base64, rot13, ...)
}

// Shell is a per-session fake shell bound to one FS. It is not safe for
// concurrent use; each player session owns its own Shell.
type Shell struct {
	fs       *FS
	cwd      string
	home     string
	user     string
	host     string
	tools    map[string]bool
	history  []string
	unknowns int
	// OnRead is called with the absolute path of every file a player
	// explicitly reads (cat/head/tail/less). Used for evidence counting and
	// collectible triggers.
	OnRead func(abs string)
}

// OptionalTools are commands that missions must enable explicitly.
var OptionalTools = []string{"base64", "rot13", "rev", "hex", "caesar"}

type input struct {
	text string
	ok   bool
}

type cmdFunc func(s *Shell, args []string, in input) (string, error)

var commands map[string]cmdFunc

func init() {
	commands = map[string]cmdFunc{
		"ls":       (*Shell).cmdLs,
		"cd":       (*Shell).cmdCd,
		"pwd":      func(s *Shell, _ []string, _ input) (string, error) { return s.cwd, nil },
		"cat":      (*Shell).cmdCat,
		"less":     (*Shell).cmdCat,
		"more":     (*Shell).cmdCat,
		"find":     (*Shell).cmdFind,
		"grep":     (*Shell).cmdGrep,
		"head":     func(s *Shell, a []string, in input) (string, error) { return s.cmdHeadTail(a, in, true) },
		"tail":     func(s *Shell, a []string, in input) (string, error) { return s.cmdHeadTail(a, in, false) },
		"wc":       (*Shell).cmdWc,
		"sort":     (*Shell).cmdSort,
		"uniq":     (*Shell).cmdUniq,
		"echo":     func(_ *Shell, a []string, _ input) (string, error) { return strings.Join(a, " "), nil },
		"tree":     (*Shell).cmdTree,
		"whoami":   func(s *Shell, _ []string, _ input) (string, error) { return s.user, nil },
		"hostname": func(s *Shell, _ []string, _ input) (string, error) { return s.host, nil },
		"date": func(_ *Shell, _ []string, _ input) (string, error) {
			return time.Now().UTC().Format("Mon Jan 2 15:04:05 UTC 2006") + "  (time is a construct, deadlines are not)", nil
		},
		"uname": func(_ *Shell, _ []string, _ input) (string, error) {
			return "DeadOS 4.0.4 deadport (definitely-not-linux) x86_64", nil
		},
		"id": func(s *Shell, _ []string, _ input) (string, error) {
			return fmt.Sprintf("uid=404(%s) gid=404(employees) groups=404(employees),1337(snack-committee)", s.user), nil
		},
		"help":    (*Shell).cmdHelp,
		"history": (*Shell).cmdHistory,
		"man": func(_ *Shell, _ []string, _ input) (string, error) {
			return "", errors.New("no manual entry. There is no manual. There was never a manual. Try 'help'")
		},
		// optional tools
		"base64": (*Shell).cmdBase64,
		"rot13":  (*Shell).cmdRot13,
		"rev":    (*Shell).cmdRev,
		"hex":    (*Shell).cmdHex,
		"caesar": (*Shell).cmdCaesar,
	}
}

// NewShell creates a shell over fs.
func NewShell(fs *FS, opt Options) *Shell {
	home := Clean("/", opt.Home)
	if n, err := fs.Lookup(home); err != nil || !n.Dir {
		home = "/"
	}
	s := &Shell{
		fs:    fs,
		cwd:   home,
		home:  home,
		user:  opt.User,
		host:  opt.Host,
		tools: map[string]bool{},
	}
	if s.user == "" {
		s.user = "employee"
	}
	if s.host == "" {
		s.host = "deadport"
	}
	for _, t := range opt.Tools {
		s.tools[t] = true
	}
	return s
}

// Cwd returns the current directory.
func (s *Shell) Cwd() string { return s.cwd }

// FS returns the shell's filesystem.
func (s *Shell) FS() *FS { return s.fs }

// History returns previously entered lines (oldest first).
func (s *Shell) History() []string { return s.history }

// Prompt renders a bash-like prompt.
func (s *Shell) Prompt() string {
	dir := s.cwd
	if dir == s.home && s.home != "/" {
		dir = "~"
	} else if s.home != "/" && strings.HasPrefix(dir, s.home+"/") {
		dir = "~" + strings.TrimPrefix(dir, s.home)
	}
	return fmt.Sprintf("%s@%s:%s$ ", s.user, s.host, dir)
}

// Exec runs one command line.
func (s *Shell) Exec(line string) Result {
	line = strings.TrimSpace(line)
	if line == "" {
		return Result{}
	}
	s.history = append(s.history, line)
	if len(s.history) > MaxHistory {
		s.history = s.history[len(s.history)-MaxHistory:]
	}

	if r, ok := secretCommand(line); ok {
		return r
	}

	pipe, err := Parse(line)
	if err != nil {
		return Result{Output: "deadport: " + err.Error(), Error: true}
	}
	if len(pipe) == 0 {
		return Result{}
	}

	if len(pipe) == 1 {
		if r, ok := s.meta(pipe[0], line); ok {
			return r
		}
	}

	var in input
	for _, stage := range pipe {
		name := stage[0]
		if r, ok := s.meta(stage, line); ok && len(pipe) > 1 {
			_ = r
			return Result{Output: name + ": can't be used in a pipe. It has commitment issues", Error: true}
		}
		out, err := s.run(name, stage[1:], in)
		if err != nil {
			return Result{Output: name + ": " + err.Error(), Error: true}
		}
		in = input{text: out, ok: true}
	}
	return Result{Output: capLines(in.text)}
}

// meta handles commands that the game (not the filesystem) cares about.
func (s *Shell) meta(args []string, raw string) (Result, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(raw, args[0]))
	switch args[0] {
	case "clear", "cls":
		return Result{Action: ActClear}, true
	case "exit", "quit", "logout":
		return Result{Action: ActExit}, true
	case "submit", "answer":
		if len(args) < 2 {
			return Result{Output: "usage: submit <answer>", Error: true}, true
		}
		return Result{Action: ActSubmit, Arg: strings.Join(args[1:], " ")}, true
	case "hint", "hints":
		return Result{Action: ActHint}, true
	case "objective", "story", "brief", "mission":
		return Result{Action: ActObjective}, true
	case "/chat", "chat", "say":
		if rest == "" {
			return Result{Output: "usage: /chat <message>", Error: true}, true
		}
		return Result{Action: ActChat, Arg: rest}, true
	case "/fix", "fix":
		return Result{Action: ActFix}, true
	}
	return Result{}, false
}

func (s *Shell) run(name string, args []string, in input) (string, error) {
	fn, ok := commands[name]
	if ok {
		for _, t := range OptionalTools {
			if t == name && !s.tools[name] {
				return "", errors.New("not installed on this machine. The IT budget was spent on a beanbag")
			}
		}
		return fn(s, args, in)
	}
	if msg, ok := realWorldCommands[name]; ok {
		return "", errors.New(msg)
	}
	s.unknowns++
	return "", errors.New(unknownCommandMessages[(s.unknowns-1)%len(unknownCommandMessages)])
}

var unknownCommandMessages = []string{
	"command not found.\nMaybe Bob knows it.\nBob does not know anything.",
	"command not found.\nIt has been added to the backlog. The backlog is on fire.",
	"command not found.\nHave you tried turning it off and on again? (type 'help')",
}

var realWorldCommands = map[string]string{
	"rm":       "read-only filesystem. You cannot delete evidence. That's Bob's job",
	"mv":       "read-only filesystem. Things stay where Bob left them. Forever",
	"cp":       "read-only filesystem. Copying is how Bob got into this mess",
	"mkdir":    "read-only filesystem. No new folders. We have enough folders",
	"touch":    "read-only filesystem. Look, don't touch",
	"chmod":    "permission to change permissions denied",
	"chown":    "you can't own things here. You can barely own a stapler",
	"vim":      "vim is not installed. You'd never get out anyway",
	"vi":       "vi is not installed. You'd never get out anyway",
	"nano":     "nano is not installed. Use cat, like our ancestors",
	"emacs":    "emacs is not installed. It's an operating system, and we already have one (barely)",
	"bash":     "no shells here. This is a game. You're already inside the only shell you'll get",
	"sh":       "no shells here. This is a game. You're already inside the only shell you'll get",
	"zsh":      "no shells here. Not even fancy ones",
	"python":   "python is not installed. A python was installed once. It escaped",
	"python3":  "python is not installed. A python was installed once. It escaped",
	"node":     "node is not installed. node_modules ate the last disk",
	"ssh":      "you're already in. Going deeper would require a totem",
	"curl":     "the internet is closed. Bob unplugged it to charge his phone",
	"wget":     "the internet is closed. Bob unplugged it to charge his phone",
	"ping":     "pong. (that's all you get)",
	"apt":      "package manager locked. Bob is installing Minecraft",
	"apt-get":  "package manager locked. Bob is installing Minecraft",
	"git":      "fatal: not a git repository (or any of the parent directories). Also not a real computer",
	"ps":       "ps is on vacation. Look for a processes file instead",
	"top":      "top: you are not at the top. Check the leaderboard",
	"kill":     "violence is never the answer. Unless it's a PID in a mission. Then submit it",
	"shutdown": "only Bob can shut down production, and he does it by accident",
	"reboot":   "rebooting would fix it, and we can't have that",
	"exec":     "absolutely not",
	"eval":     "absolutely not",
	"nc":       "netcat is not available. The cat is, though. Try 'cat'",
	"nmap":     "the network has been scanned. It is fine. It is not fine",
	"su":       "su: authentication failure. You're not even super",
	"passwd":   "passwords are managed by Bob. That's the problem",
	"exploit":  "no",
}

func secretCommand(line string) (Result, bool) {
	norm := strings.ToLower(strings.Join(strings.Fields(line), " "))
	switch {
	case norm == "sudo" || strings.HasPrefix(norm, "sudo ") && !strings.Contains(norm, "rm -rf") && !strings.Contains(norm, "sandwich"):
		return Result{Output: "Nice try.\n\nYou don't have sudo.\n\nYou don't even have a job."}, true
	case norm == "hack nasa" || strings.HasPrefix(norm, "hack ") || norm == "hack":
		return Result{Output: "Command rejected.\n\nBro thinks this is Mr. Robot. 💀", Secret: "mr_robot"}, true
	case strings.Contains(norm, "rm -rf /") || strings.Contains(norm, "rm -fr /") || strings.Contains(norm, "rm -rf ~") || strings.Contains(norm, "rm -rf *"):
		return Result{Output: "Absolutely not.\n\nThis is a game.\n\nCalm down, Mr. Robot.", Secret: "mr_robot"}, true
	case strings.Contains(norm, ":(){") || strings.Contains(norm, ":() {"):
		return Result{Output: "A fork bomb. In this economy?\n\nThe forks have been confiscated.", Secret: "mr_robot"}, true
	case strings.Contains(norm, "make me a sandwich"):
		if strings.HasPrefix(norm, "sudo") {
			return Result{Output: "Okay.\n\n🥪\n\n(It's Bob's. Don't tell him.)"}, true
		}
		return Result{Output: "What? Make it yourself."}, true
	case norm == "xyzzy":
		return Result{Output: "Nothing happens.\n\nA hollow voice says \"Bob\"."}, true
	case norm == "quack":
		return Result{Output: "The duck nods approvingly. It still won't tell you anything."}, true
	case norm == "hello" || norm == "hi":
		return Result{Output: "Hi. This is a terminal. It does not do small talk. Try 'help'."}, true
	case norm == "please":
		return Result{Output: "Manners won't get you root. But thank you."}, true
	case norm == "coffee" || norm == "make coffee" || norm == "brew coffee":
		return Result{Output: "418 I'm a teapot."}, true
	case norm == "42":
		return Result{Output: "Correct, but for the wrong question."}, true
	}
	return Result{}, false
}

func capLines(s string) string {
	if strings.Count(s, "\n") < MaxOutputLines {
		return s
	}
	lines := strings.SplitN(s, "\n", MaxOutputLines+1)
	return strings.Join(lines[:MaxOutputLines], "\n") + fmt.Sprintf("\n... output truncated at %d lines. Try grep, head or tail.", MaxOutputLines)
}

// splitFlags separates "-x"-style flags from positional arguments.
// Flags taking a value (listed in withValue) consume the following argument.
func splitFlags(args []string, withValue string) (flags map[string]string, pos []string) {
	flags = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && !isNumber(a) {
			if strings.HasPrefix(a, "--") {
				flags[strings.TrimPrefix(a, "--")] = ""
				continue
			}
			letters := a[1:]
			for j, c := range letters {
				f := string(c)
				if strings.ContainsRune(withValue, c) {
					if j+1 < len(letters) {
						flags[f] = letters[j+1:]
					} else if i+1 < len(args) {
						flags[f] = args[i+1]
						i++
					} else {
						flags[f] = ""
					}
					break
				}
				flags[f] = ""
			}
			continue
		}
		pos = append(pos, a)
	}
	return flags, pos
}

func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func has(flags map[string]string, names ...string) bool {
	for _, n := range names {
		if _, ok := flags[n]; ok {
			return true
		}
	}
	return false
}

func (s *Shell) resolve(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		p = s.home + strings.TrimPrefix(p, "~")
	}
	return Clean(s.cwd, p)
}

// readFile reads a file for a player and fires OnRead.
func (s *Shell) readFile(p string, notify bool) (string, string, error) {
	abs := s.resolve(p)
	content, err := s.fs.ReadFile(abs)
	if err != nil {
		return "", abs, fmt.Errorf("%s: %w", p, err)
	}
	if notify && s.OnRead != nil {
		s.OnRead(abs)
	}
	return content, abs, nil
}

// source returns either stdin or the content of the files/text given.
func (s *Shell) source(pos []string, in input, notify bool) (string, error) {
	if len(pos) == 0 {
		if in.ok {
			return in.text, nil
		}
		return "", errors.New("missing file operand. Read what? From where?")
	}
	var parts []string
	for _, p := range pos {
		c, _, err := s.readFile(p, notify)
		if err != nil {
			return "", err
		}
		parts = append(parts, c)
	}
	return strings.Join(parts, "\n"), nil
}

func (s *Shell) cmdLs(args []string, _ input) (string, error) {
	flags, pos := splitFlags(args, "")
	all := has(flags, "a", "A")
	long := has(flags, "l")
	if len(pos) == 0 {
		pos = []string{"."}
	}
	var out []string
	for i, p := range pos {
		abs := s.resolve(p)
		n, err := s.fs.Lookup(abs)
		if err != nil {
			return "", fmt.Errorf("cannot access '%s': %w", p, err)
		}
		if len(pos) > 1 {
			if i > 0 {
				out = append(out, "")
			}
			out = append(out, p+":")
		}
		if !n.Dir {
			out = append(out, formatEntry(n, long))
			continue
		}
		for _, c := range n.Children() {
			if !all && strings.HasPrefix(c.Name, ".") {
				continue
			}
			out = append(out, formatEntry(c, long))
		}
	}
	return strings.Join(out, "\n"), nil
}

func formatEntry(n *Node, long bool) string {
	name := n.Name
	if n.Dir {
		name += "/"
	}
	if !long {
		return name
	}
	if n.Dir {
		return fmt.Sprintf("drwxr-xr-x  bob  %6s  %s", "-", name)
	}
	return fmt.Sprintf("-rw-r--r--  bob  %6d  %s", len(n.Content), name)
}

func (s *Shell) cmdCd(args []string, _ input) (string, error) {
	target := s.home
	if len(args) > 0 {
		if args[0] == "-" {
			return "", errors.New("there is no going back. Only forward. Like a career in IT")
		}
		target = s.resolve(args[0])
	}
	n, err := s.fs.Lookup(target)
	if err != nil {
		return "", fmt.Errorf("%s: %w", args[0], err)
	}
	if !n.Dir {
		return "", fmt.Errorf("%s: %w", args[0], ErrNotDir)
	}
	s.cwd = target
	return "", nil
}

func (s *Shell) cmdCat(args []string, in input) (string, error) {
	_, pos := splitFlags(args, "")
	return s.source(pos, in, true)
}

func (s *Shell) cmdHeadTail(args []string, in input, head bool) (string, error) {
	n := 10
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-n" && i+1 < len(args):
			v, err := strconv.Atoi(strings.TrimPrefix(args[i+1], "+"))
			if err != nil {
				return "", fmt.Errorf("invalid number of lines: '%s'", args[i+1])
			}
			n = v
			i++
		case strings.HasPrefix(a, "-n") && len(a) > 2:
			v, err := strconv.Atoi(a[2:])
			if err != nil {
				return "", fmt.Errorf("invalid number of lines: '%s'", a[2:])
			}
			n = v
		case len(a) > 1 && a[0] == '-' && isNumber(a[1:]):
			v, _ := strconv.Atoi(a[1:])
			n = v
		default:
			rest = append(rest, a)
		}
	}
	if n < 0 {
		n = -n
	}
	text, err := s.source(rest, in, true)
	if err != nil {
		return "", err
	}
	lines := strings.Split(text, "\n")
	if n >= len(lines) {
		return text, nil
	}
	if head {
		return strings.Join(lines[:n], "\n"), nil
	}
	return strings.Join(lines[len(lines)-n:], "\n"), nil
}

func (s *Shell) cmdFind(args []string, _ input) (string, error) {
	var (
		roots   []string
		name    string
		iname   bool
		typ     string
		pending string
	)
	for _, a := range args {
		if pending != "" {
			switch pending {
			case "-name":
				name = a
			case "-iname":
				name, iname = strings.ToLower(a), true
			case "-type":
				typ = a
			}
			pending = ""
			continue
		}
		switch a {
		case "-name", "-iname", "-type":
			pending = a
		default:
			if strings.HasPrefix(a, "-") {
				return "", fmt.Errorf("unknown predicate '%s'. Supported: -name, -iname, -type f|d", a)
			}
			roots = append(roots, a)
		}
	}
	if pending != "" {
		return "", fmt.Errorf("missing argument to '%s'", pending)
	}
	if typ != "" && typ != "f" && typ != "d" {
		return "", fmt.Errorf("unknown type '%s'. Use f or d", typ)
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	var out []string
	for _, r := range roots {
		abs := s.resolve(r)
		err := s.fs.Walk(abs, func(p string, n *Node) {
			if typ == "f" && n.Dir || typ == "d" && !n.Dir {
				return
			}
			if name != "" {
				base := n.Name
				if iname {
					base = strings.ToLower(base)
				}
				if ok, _ := path.Match(name, base); !ok {
					return
				}
			}
			out = append(out, p)
		})
		if err != nil {
			return "", fmt.Errorf("'%s': %w", r, err)
		}
	}
	return strings.Join(out, "\n"), nil
}

func (s *Shell) cmdGrep(args []string, in input) (string, error) {
	flags, pos := splitFlags(args, "e")
	pattern := ""
	if v, ok := flags["e"]; ok {
		pattern = v
	} else {
		if len(pos) == 0 {
			return "", errors.New("usage: grep [-i -r -n -v -c -l] PATTERN [FILE...]")
		}
		pattern, pos = pos[0], pos[1:]
	}
	if len(pattern) > 200 {
		return "", errors.New("pattern too long")
	}
	expr := pattern
	if has(flags, "F") {
		expr = regexp.QuoteMeta(pattern)
	}
	if has(flags, "w") {
		expr = `\b(?:` + expr + `)\b`
	}
	if has(flags, "i") {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		// fall back to a literal match: players type things like "file(1)".
		lit := regexp.QuoteMeta(pattern)
		if has(flags, "i") {
			lit = "(?i)" + lit
		}
		re = regexp.MustCompile(lit)
	}
	invert := has(flags, "v")
	count := has(flags, "c")
	number := has(flags, "n")
	listOnly := has(flags, "l")
	recursive := has(flags, "r", "R")

	type src struct{ name, text string }
	var sources []src
	if len(pos) == 0 {
		switch {
		case in.ok:
			sources = append(sources, src{"", in.text})
		case recursive:
			pos = []string{"."}
		default:
			return "", errors.New("no input. Grep what? From where? (try: grep PATTERN FILE)")
		}
	}
	for _, p := range pos {
		abs := s.resolve(p)
		n, err := s.fs.Lookup(abs)
		if err != nil {
			return "", fmt.Errorf("%s: %w", p, err)
		}
		if n.Dir {
			if !recursive {
				return "", fmt.Errorf("%s: Is a directory (use grep -r)", p)
			}
			_ = s.fs.Walk(abs, func(fp string, fn *Node) {
				if !fn.Dir {
					sources = append(sources, src{fp, fn.Content})
				}
			})
			continue
		}
		label := p
		if recursive {
			label = abs
		}
		sources = append(sources, src{label, n.Content})
	}
	prefix := len(sources) > 1 || recursive
	if has(flags, "h") {
		prefix = false
	}
	var out []string
	for _, sc := range sources {
		matches := 0
		for i, line := range strings.Split(sc.text, "\n") {
			if re.MatchString(line) == invert {
				continue
			}
			matches++
			if count || listOnly {
				continue
			}
			var b strings.Builder
			if prefix && sc.name != "" {
				b.WriteString(sc.name + ":")
			}
			if number {
				b.WriteString(strconv.Itoa(i+1) + ":")
			}
			b.WriteString(line)
			out = append(out, b.String())
			if len(out) > MaxOutputLines {
				break
			}
		}
		switch {
		case listOnly && matches > 0:
			out = append(out, sc.name)
		case count && prefix && sc.name != "":
			out = append(out, fmt.Sprintf("%s:%d", sc.name, matches))
		case count:
			out = append(out, strconv.Itoa(matches))
		}
	}
	return strings.Join(out, "\n"), nil
}

func (s *Shell) cmdWc(args []string, in input) (string, error) {
	flags, pos := splitFlags(args, "")
	text, err := s.source(pos, in, false)
	if err != nil {
		return "", err
	}
	lines := 0
	if text != "" {
		lines = strings.Count(text, "\n") + 1
	}
	words := len(strings.Fields(text))
	chars := len([]rune(text))
	switch {
	case has(flags, "l"):
		return strconv.Itoa(lines), nil
	case has(flags, "w"):
		return strconv.Itoa(words), nil
	case has(flags, "c", "m"):
		return strconv.Itoa(chars), nil
	}
	return fmt.Sprintf("%d lines  %d words  %d chars", lines, words, chars), nil
}

func (s *Shell) cmdSort(args []string, in input) (string, error) {
	flags, pos := splitFlags(args, "")
	text, err := s.source(pos, in, false)
	if err != nil {
		return "", err
	}
	lines := strings.Split(text, "\n")
	if has(flags, "n") {
		sort.SliceStable(lines, func(i, j int) bool { return leadingNumber(lines[i]) < leadingNumber(lines[j]) })
	} else {
		sort.Strings(lines)
	}
	if has(flags, "r") {
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
	}
	if has(flags, "u") {
		lines = dedupe(lines, false)
	}
	return strings.Join(lines, "\n"), nil
}

func leadingNumber(s string) float64 {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0
	}
	return v
}

func dedupe(lines []string, withCount bool) []string {
	var out []string
	for i := 0; i < len(lines); {
		j := i
		for j < len(lines) && lines[j] == lines[i] {
			j++
		}
		if withCount {
			out = append(out, fmt.Sprintf("%7d %s", j-i, lines[i]))
		} else {
			out = append(out, lines[i])
		}
		i = j
	}
	return out
}

func (s *Shell) cmdUniq(args []string, in input) (string, error) {
	flags, pos := splitFlags(args, "")
	text, err := s.source(pos, in, false)
	if err != nil {
		return "", err
	}
	return strings.Join(dedupe(strings.Split(text, "\n"), has(flags, "c")), "\n"), nil
}

func (s *Shell) cmdTree(args []string, _ input) (string, error) {
	flags, pos := splitFlags(args, "")
	all := has(flags, "a")
	root := "."
	if len(pos) > 0 {
		root = pos[0]
	}
	abs := s.resolve(root)
	n, err := s.fs.Lookup(abs)
	if err != nil {
		return "", fmt.Errorf("%s: %w", root, err)
	}
	if !n.Dir {
		return "", fmt.Errorf("%s: %w", root, ErrNotDir)
	}
	out := []string{abs}
	dirs, files := 0, 0
	var walk func(n *Node, prefix string, depth int)
	walk = func(n *Node, prefix string, depth int) {
		var kids []*Node
		for _, c := range n.Children() {
			if all || !strings.HasPrefix(c.Name, ".") {
				kids = append(kids, c)
			}
		}
		for i, c := range kids {
			branch, next := "├── ", "│   "
			if i == len(kids)-1 {
				branch, next = "└── ", "    "
			}
			label := c.Name
			if c.Dir {
				label += "/"
				dirs++
			} else {
				files++
			}
			out = append(out, prefix+branch+label)
			if c.Dir && depth < 8 {
				walk(c, prefix+next, depth+1)
			}
		}
	}
	walk(n, "", 1)
	out = append(out, "", fmt.Sprintf("%d directories, %d files", dirs, files))
	return strings.Join(out, "\n"), nil
}

func (s *Shell) cmdHistory(_ []string, _ input) (string, error) {
	var out []string
	for i, h := range s.history {
		out = append(out, fmt.Sprintf("%4d  %s", i+1, h))
	}
	return strings.Join(out, "\n"), nil
}

func (s *Shell) cmdHelp(_ []string, _ input) (string, error) {
	var b strings.Builder
	b.WriteString(`EXPLORE
  ls [-a] [-l] [path]      list files (-a shows hidden dotfiles)
  cd <path>                change directory (cd .. goes up, cd ~ goes home)
  pwd                      print current directory
  tree [-a] [path]         draw the directory tree
  cat <file>               print a file
  head|tail [-n N] <file>  first/last N lines
  find [path] -name "*x*"  search for files by name (includes hidden)
  grep [-i -r -n -v -c] PATTERN [file|dir]
  wc [-l]  sort [-r -n -u]  uniq [-c]  echo  history  whoami
  Pipes work: grep error app.log | wc -l
`)
	var tools []string
	for _, t := range OptionalTools {
		if s.tools[t] {
			tools = append(tools, t)
		}
	}
	if len(tools) > 0 {
		b.WriteString("\nTOOLS INSTALLED ON THIS MACHINE\n")
		for _, t := range tools {
			b.WriteString("  " + toolHelp[t] + "\n")
		}
	}
	b.WriteString(`
GAME
  objective                show the mission briefing again
  hint                     reveal the next hint (costs XP)
  submit <answer>          submit your answer
  /chat <message>          talk to everyone online
  clear                    clear the screen
  exit                     leave this terminal (Esc also works)`)
	return b.String(), nil
}

var toolHelp = map[string]string{
	"base64": "base64 [-d] [file|text]  encode / decode base64",
	"rot13":  "rot13 [file|text]        rotate letters by 13",
	"rev":    "rev [file|text]          reverse each line",
	"hex":    "hex [-d] [file|text]     encode / decode hex",
	"caesar": "caesar <shift> [file|text]  shift letters (negative shifts go back)",
}

// toolSource reads tool input: stdin, a single existing file, or literal text.
func (s *Shell) toolSource(pos []string, in input) (string, error) {
	if len(pos) == 0 {
		if in.ok {
			return in.text, nil
		}
		return "", errors.New("no input. Pipe something in or pass text/a file")
	}
	if len(pos) == 1 {
		if n, err := s.fs.Lookup(s.resolve(pos[0])); err == nil && !n.Dir {
			return n.Content, nil
		}
	}
	return strings.Join(pos, " "), nil
}

// decodeLines tries to decode every line (or every token on a line) and
// returns only the parts that decoded to printable text.
func decodeLines(text string, dec func(string) (string, bool)) (string, bool) {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if d, ok := dec(line); ok && Printable(d) {
			out = append(out, d)
			continue
		}
		for _, tok := range strings.Fields(line) {
			if len(tok) < 4 {
				continue
			}
			if d, ok := dec(strings.Trim(tok, "\"'`,;()[]")); ok && Printable(d) {
				out = append(out, d)
			}
		}
	}
	return strings.Join(out, "\n"), len(out) > 0
}

func (s *Shell) cmdBase64(args []string, in input) (string, error) {
	flags, pos := splitFlags(args, "")
	text, err := s.toolSource(pos, in)
	if err != nil {
		return "", err
	}
	if has(flags, "d", "decode") {
		out, ok := decodeLines(text, Base64Decode)
		if !ok {
			return "", errors.New("invalid input. That's not base64, that's just shouting")
		}
		return out, nil
	}
	return Base64Encode(text), nil
}

func (s *Shell) cmdHex(args []string, in input) (string, error) {
	flags, pos := splitFlags(args, "")
	text, err := s.toolSource(pos, in)
	if err != nil {
		return "", err
	}
	if has(flags, "d", "decode", "r") {
		out, ok := decodeLines(text, HexDecode)
		if !ok {
			return "", errors.New("invalid input. That's not hex, that's a cry for help")
		}
		return out, nil
	}
	return HexEncode(text), nil
}

func (s *Shell) cmdRot13(args []string, in input) (string, error) {
	_, pos := splitFlags(args, "")
	text, err := s.toolSource(pos, in)
	if err != nil {
		return "", err
	}
	return ROT13(text), nil
}

func (s *Shell) cmdRev(args []string, in input) (string, error) {
	_, pos := splitFlags(args, "")
	text, err := s.toolSource(pos, in)
	if err != nil {
		return "", err
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = Reverse(l)
	}
	return strings.Join(lines, "\n"), nil
}

func (s *Shell) cmdCaesar(args []string, in input) (string, error) {
	if len(args) == 0 {
		return "", errors.New("usage: caesar <shift> [file|text]")
	}
	shift, err := strconv.Atoi(args[0])
	if err != nil {
		return "", errors.New("shift must be a number, like 3 or -3")
	}
	text, err := s.toolSource(args[1:], in)
	if err != nil {
		return "", err
	}
	return Caesar(text, shift), nil
}

// Complete performs tab completion on the last token of line. It returns the
// new line and, when ambiguous, the list of candidates.
func (s *Shell) Complete(line string) (string, []string) {
	idx := strings.LastIndexAny(line, " |")
	prefix, token := line[:idx+1], line[idx+1:]
	first := strings.TrimSpace(prefix) == "" || strings.HasSuffix(strings.TrimSpace(prefix), "|")

	var cands []string
	if first && !strings.Contains(token, "/") {
		names := []string{"submit", "hint", "objective", "clear", "exit"}
		for name := range commands {
			isTool := false
			for _, t := range OptionalTools {
				if t == name && !s.tools[name] {
					isTool = true
				}
			}
			if !isTool {
				names = append(names, name)
			}
		}
		for _, n := range names {
			if strings.HasPrefix(n, token) {
				cands = append(cands, n+" ")
			}
		}
	} else {
		dirPart, base := "", token
		if i := strings.LastIndex(token, "/"); i >= 0 {
			dirPart, base = token[:i+1], token[i+1:]
		}
		lookup := dirPart
		if lookup == "" {
			lookup = "."
		}
		n, err := s.fs.Lookup(s.resolve(lookup))
		if err != nil || !n.Dir {
			return line, nil
		}
		for _, c := range n.Children() {
			if strings.HasPrefix(c.Name, ".") && !strings.HasPrefix(base, ".") {
				continue
			}
			if strings.HasPrefix(c.Name, base) {
				suffix := " "
				if c.Dir {
					suffix = "/"
				}
				cands = append(cands, dirPart+c.Name+suffix)
			}
		}
	}
	sort.Strings(cands)
	switch len(cands) {
	case 0:
		return line, nil
	case 1:
		return prefix + cands[0], nil
	}
	common := cands[0]
	for _, c := range cands[1:] {
		for !strings.HasPrefix(c, common) {
			common = common[:len(common)-1]
		}
	}
	shown := make([]string, len(cands))
	for i, c := range cands {
		shown[i] = strings.TrimSpace(path.Base(strings.TrimSuffix(c, "/")))
		if strings.HasSuffix(c, "/") {
			shown[i] += "/"
		}
	}
	return prefix + common, shown
}
