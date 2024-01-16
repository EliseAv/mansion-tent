package tent

import (
	"bufio"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type regexpDispatch struct {
	callback func([]string)
	regex    regexp.Regexp
}

type sitter struct {
	saveName          string
	retry             bool
	proc              *exec.Cmd
	stdout            io.ReadCloser
	stderr            io.ReadCloser
	stdin             io.WriteCloser
	players           map[string]bool
	startedAt         time.Time
	nextShutdownCheck time.Time
	regexps           []regexpDispatch
}

var Sitter sitter

func init() {
	Sitter.saveName = "saves/world.zip"
	Sitter.players = make(map[string]bool)
	Sitter.startedAt = time.Now()
	Sitter.nextShutdownCheck = Sitter.startedAt.Add(15 * time.Minute)
	Sitter.regexps = []regexpDispatch{
		{Sitter.onInGame, *regexp.MustCompile(`^\s*\d+\.\d+ Info ServerMultiplayerManager\.cpp:\d+: updateTick\(tick=(\d+)\) changing state from\(CreatingGame\) to\(InGame\)$`)},
		{Sitter.onJoined, *regexp.MustCompile(`^....-..-.. ..:..:.. \[JOIN] (.+) joined the game$`)},
		{Sitter.onLeft, *regexp.MustCompile(`^....-..-.. ..:..:.. \[LEAVE] (.+) left the game$`)},
		{Sitter.onSaved, *regexp.MustCompile(`^\s*\d+\.\d+ Info AppManagerStates\.cpp:\d+: Saving finished$`)},
		{Sitter.onQuitCmd, *regexp.MustCompile(`^\s*\d+\.\d+ Quitting: remote-quit.$`)},
	}
}

func (s *sitter) Run() {
	for s.retry = true; s.retry; {
		s.launch()
		go s.watchForShutdown()
		go io.Copy(s.stdin, os.Stdin)
		go s.parseAndPass(os.Stderr, s.stderr, "31")
		s.parseAndPass(os.Stdout, s.stdout, "32")
	}
}

func (s *sitter) launch() {
	var err error
	log.Printf("\033[1;32mLaunching game with save %s\033[0m\n", s.saveName)
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
		log.Printf("\033[1;31mWorking directory: %s\033[0m\n", cwd)
		panic(err)
	}
}

func (s *sitter) parseAndPass(out *os.File, in io.ReadCloser, color string) {
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
	}
}

func (s *sitter) onInGame(_ []string) {
	s.startedAt = time.Now()
	s.bumpShutdownCheck()
	go Hooks.onLaunched()
}

func (s *sitter) onSaved(_ []string) {
	go Hooks.onSaved()
}

func (s *sitter) onJoined(match []string) {
	s.players[match[1]] = true
	go Hooks.onJoined(match[1])
}

func (s *sitter) onLeft(match []string) {
	delete(s.players, match[1])
	s.bumpShutdownCheck()
	go Hooks.onLeft(match[1])
	if len(s.players) == 0 {
		go Hooks.onDrained(time.Until(s.nextShutdownCheck))
	}
}

func (s *sitter) onQuitCmd(_ []string) {
	s.retry = false
}

func (s *sitter) bumpShutdownCheck() {
	next := time.Now().Add(3 * time.Minute)
	if s.nextShutdownCheck.Before(next) {
		s.nextShutdownCheck = next
	}
}

func (s *sitter) watchForShutdown() {
	for wait := time.Until(s.nextShutdownCheck); wait > 0; wait = time.Until(s.nextShutdownCheck) {
		log.Printf("\033[1;34mWaiting %s for shutdown check...\033[0m\n", wait)
		time.Sleep(wait)
		if len(s.players) > 0 {
			// i know this looks like a busy wait, but it's minutes per loop; it'll be fine
			s.bumpShutdownCheck()
		}
	}
	// time to shut down!
	Hooks.onQuit()
	s.stdin.Write([]byte("/quit\n"))
	s.retry = false
}
