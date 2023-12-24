package tent

import (
	"archive/tar"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/xi2/xz"
)

type tent struct {
	aws      *session.Session
	s3       *s3.S3
	s3folder url.URL
}

var Tent tent

func init() {
	Tent.aws = session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	Tent.aws.Config.Region = aws.String(os.Getenv("AWS_REGION"))
	Tent.s3 = s3.New(Tent.aws)

	parsed, err := url.Parse(os.Getenv("S3_FOLDER_URL"))
	if err != nil {
		panic(err)
	}
	parsed.Path = strings.Trim(parsed.Path, "/")
	Tent.s3folder = *parsed
	Tent.s3folder.Path = strings.Trim(Tent.s3folder.Path, "/")
}

func (t *tent) Run() {
	t.downloadGame()
	t.copyFiles()
	os.Chdir("factorio")
	Sitter.Run()
}

func (t *tent) downloadGame() {
	// check if we need to do this
	_, err := os.Stat("factorio/bin/x64/factorio")
	if err == nil {
		return // game is already downloaded
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	// download
	url := "https://www.factorio.com/get-download/latest/headless/linux64"
	log.Printf("Downloading game from %s\n", url)
	download, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer download.Body.Close()
	decompress, err := xz.NewReader(download.Body, 0)
	if err != nil {
		panic(err)
	}
	// we'll unpack everything as we download it
	unpack := tar.NewReader(decompress)
	for {
		header, err := unpack.Next()
		if err == io.EOF {
			break // end of tar archive
		} else if err != nil {
			panic(err)
		}
		log.Printf("Unpacking %s\n", header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(header.Name, 0755)
			if err != nil {
				panic(err)
			}
		case tar.TypeReg:
			func() {
				file, err := os.Create(header.Name)
				if err != nil {
					panic(err)
				}
				defer file.Close()
				_, err = io.Copy(file, unpack)
				if err != nil {
					panic(err)
				}
			}()
		}
	}
}

func (t *tent) copyFiles() {
	// check if we need to do this
	_, err := os.Stat("factorio/saves")
	if err == nil {
		return // files are already copied over
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	// get the directory of the executable
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	executableDir := filepath.Dir(executable)
	// traverse executable dir and copy everything to working directory
	filepath.Walk(executableDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			panic(err)
		}
		relativePath := strings.TrimPrefix(path, executableDir)
		if relativePath == "" {
			log.Println("Skipping root directory")
		} else if info.IsDir() {
			log.Printf("Creating directory %s\n", relativePath)
			err = os.MkdirAll(relativePath, info.Mode())
			if err != nil {
				panic(err)
			}
		} else if info.Mode().IsRegular() {
			log.Printf("Copying %s\n", relativePath)
			srcFile, err := os.Open(path)
			if err != nil {
				panic(err)
			}
			defer srcFile.Close()
			dstFile, err := os.OpenFile(relativePath, os.O_CREATE|os.O_WRONLY, info.Mode())
			if err != nil {
				panic(err)
			}
			defer dstFile.Close()
			_, err = io.Copy(dstFile, srcFile)
			if err != nil {
				panic(err)
			}
		} else {
			log.Printf("Skipping %s (mode %o)\n", relativePath, info.Mode())
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}
