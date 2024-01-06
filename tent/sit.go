package tent

import (
	"bufio"
	"fmt"
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
	}
}

func (s *sitter) Run() {
	for s.retry = true; s.retry; {
		s.launch()
		go s.watchForShutdown()
		go s.parseAndPass(os.Stdout, s.stdout, "32")
		go s.parseAndPass(os.Stderr, s.stderr, "31")
		io.Copy(s.stdin, os.Stdin)
	}
}

func (s *sitter) launch() {
	log.Printf("Launching game with save %s\n", s.saveName)
	s.proc = exec.Command("bin/x64/factorio", "--start-server", s.saveName)
	err := s.proc.Start()
	if err != nil {
		cwd, _ := os.Getwd()
		log.Printf("Working directory: %s\n", cwd)
		panic(err)
	}
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
}

func (s *sitter) parseAndPass(out *os.File, in io.ReadCloser, color string) {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\n")
		for _, red := range s.regexps {
			match := red.regex.FindStringSubmatch(line)
			if match != nil {
				red.callback(match)
				break
			}
		}
		outLine := fmt.Sprintf("\033[1;%sm%s\033[0m\n", color, line)
		out.Write([]byte(outLine))
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

func (s *sitter) bumpShutdownCheck() {
	next := time.Now().Add(3 * time.Minute)
	if s.nextShutdownCheck.Before(next) {
		s.nextShutdownCheck = next
	}
}

func (s *sitter) watchForShutdown() {
	for wait := time.Until(s.nextShutdownCheck); wait > 0; wait = time.Until(s.nextShutdownCheck) {
		fmt.Printf("\033[1;34mWaiting %s for shutdown check...\033[0m\n", wait)
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
