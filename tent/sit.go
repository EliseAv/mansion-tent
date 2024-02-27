package tent

import (
	"bufio"
	"io"
	"log/slog"
	"mansionTent/share"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type regexpDispatch struct {
	callback func([]string)
	regex    regexp.Regexp
}

type sitter struct {
	hooks             *hooks
	saveName          string
	retry             bool
	proc              *exec.Cmd
	stdout            io.ReadCloser
	stderr            io.ReadCloser
	stdin             io.WriteCloser
	players           share.Set[string]
	nextShutdownCheck time.Time
	regexps           []regexpDispatch
	shutdownGrace     struct {
		initial time.Duration
		drained time.Duration
	}
}

func NewSitter(hooks *hooks) *sitter {
	s := &sitter{
		hooks:    hooks,
		saveName: "saves/world.zip",
	}
	s.nextShutdownCheck = time.Now().Add(s.shutdownGrace.initial)
	s.shutdownGrace.initial = parseFloatToMinutesOrDefault("SHUTDOWN_GRACE_INITIAL_MINUTES", 15)
	s.shutdownGrace.drained = parseFloatToMinutesOrDefault("SHUTDOWN_GRACE_DRAINED_MINUTES", 3)
	s.regexps = []regexpDispatch{
		{s.onInGame, *regexp.MustCompile(`^\s*\d+\.\d+ Info ServerMultiplayerManager\.cpp:\d+: updateTick\(\d+\) changing state from\(CreatingGame\) to\(InGame\)$`)},
		{s.onJoined, *regexp.MustCompile(`^....-..-.. ..:..:.. \[JOIN] (.+) joined the game$`)},
		{s.onLeft, *regexp.MustCompile(`^....-..-.. ..:..:.. \[LEAVE] (.+) left the game$`)},
		{s.onSaved, *regexp.MustCompile(`^\s*\d+\.\d+ Info AppManagerStates\.cpp:\d+: Saving finished$`)},
		{s.onQuitCmd, *regexp.MustCompile(`^\s*\d+\.\d+ Quitting: remote-quit.$`)},
	}
	return s
}

func parseFloatToMinutesOrDefault(key string, def float64) time.Duration {
	value := def
	str := os.Getenv(key)
	if str != "" {
		parsed, err := strconv.ParseFloat(str, 64)
		if err != nil {
			slog.Warn("Invalid float", "key", key, "value", str, "err", err)
		} else {
			value = parsed
		}
	}
	return time.Duration(value * float64(time.Minute))
}

func (s *sitter) Run() {
	for s.retry = true; s.retry; {
		s.launch()
		go s.watchForShutdown()
		go io.Copy(s.stdin, os.Stdin)
		go s.parseAndPass(os.Stderr, s.stderr)
		s.parseAndPass(os.Stdout, s.stdout)
	}
}

func (s *sitter) launch() {
	var err error
	slog.Info("Launching game", "save", s.saveName)
	s.proc = exec.Command("bin/x64/factorio", "--start-server", s.saveName)
	s.stdout, err = s.proc.StdoutPipe()
	if err != nil {
		panic(err)
	}
	s.stderr, err = s.proc.StderrPipe()
	if err != nil {
		panic(err)
	}
	s.stdin, err = s.proc.StdinPipe()
	if err != nil {
		panic(err)
	}
	err = s.proc.Start()
	if err != nil {
		cwd, _ := os.Getwd()
		slog.Info("Working directory", "cwd", cwd)
		panic(err)
	}
}

func (s *sitter) parseAndPass(out *os.File, in io.ReadCloser) {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := scanner.Text()
		for _, red := range s.regexps {
			match := red.regex.FindStringSubmatch(strings.TrimSuffix(line, "\n"))
			if match != nil {
				red.callback(match)
				break
			}
		}
		out.Write([]byte(line))
		out.Write([]byte("\n"))
	}
}

func (s *sitter) onInGame(_ []string) {
	s.nextShutdownCheck = time.Now().Add(s.shutdownGrace.initial)
	go s.hooks.onLaunched()
}

func (s *sitter) onSaved(_ []string) {
	go s.hooks.onSaved()
}

func (s *sitter) onJoined(match []string) {
	s.players.Add(match[1])
	go s.hooks.onJoined(match[1])
}

func (s *sitter) onLeft(match []string) {
	s.players.Remove(match[1])
	s.bumpShutdownCheck()
	go s.hooks.onLeft(match[1])
	if s.players.Len() == 0 {
		go s.hooks.onDrained(time.Until(s.nextShutdownCheck))
	}
}

func (s *sitter) onQuitCmd(_ []string) {
	s.retry = false
}

func (s *sitter) bumpShutdownCheck() {
	next := time.Now().Add(s.shutdownGrace.drained)
	if s.nextShutdownCheck.Before(next) {
		s.nextShutdownCheck = next
	}
}

func (s *sitter) watchForShutdown() {
	for wait := s.shutdownGrace.initial; wait > 0; wait = time.Until(s.nextShutdownCheck) {
		slog.Info("Waiting for next shutdown check", "players", s.players.Len(), "wait", wait)
		time.Sleep(wait)
		if s.players.Len() > 0 {
			// i know this looks like a busy wait, but it's minutes per loop; it'll be fine
			s.bumpShutdownCheck()
		}
	}
	// time to shut down!
	slog.Info("Shutting down")
	s.hooks.onQuit()
	s.stdin.Write([]byte("/quit\n"))
	s.retry = false
}
