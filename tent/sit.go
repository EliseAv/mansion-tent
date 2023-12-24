package tent

import (
	"io"
	"log"
	"os"
	"os/exec"
	"time"
)

type sitter struct {
	saveName          string
	retry             bool
	proc              *exec.Cmd
	stdout            io.ReadCloser
	stderr            io.ReadCloser
	stdin             io.WriteCloser
	players           map[string]bool
	shutdownSignal    chan bool
	startedAt         time.Time
	nextShutdownCheck time.Time
}

var Sitter sitter

func init() {
	Sitter.saveName = "saves/world.zip"
	Sitter.retry = true
	Sitter.players = make(map[string]bool)
	Sitter.shutdownSignal = make(chan bool)
	Sitter.startedAt = time.Now()
	Sitter.nextShutdownCheck = Sitter.startedAt.Add(5 * time.Minute)
}

func (s *sitter) Run() {
	for s.retry {
		s.launch()
		go io.Copy(os.Stdout, s.stdout)
		go io.Copy(os.Stderr, s.stderr)
		io.Copy(s.stdin, os.Stdin)
	}
}

func (s *sitter) launch() {
	log.Printf("Launching game with save %s\n", s.saveName)
	s.proc = exec.Command("bin/x64/factorio", "--start-server", s.saveName)
	err := s.proc.Start()
	if err != nil {
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
