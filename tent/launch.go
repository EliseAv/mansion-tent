package tent

import (
	"archive/tar"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/xi2/xz"
)

type launcher struct {
	aws      *session.Session
	s3       *s3.S3
	s3folder url.URL
}

var Launcher launcher

func init() {
	Launcher.aws = session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	Launcher.aws.Config.Region = aws.String(os.Getenv("AWS_REGION"))
	Launcher.s3 = s3.New(Launcher.aws)

	parsed, err := url.Parse(os.Getenv("S3_FOLDER_URL"))
	if err != nil {
		panic(err)
	}
	parsed.Path = strings.Trim(parsed.Path, "/")
	Launcher.s3folder = *parsed
	Launcher.s3folder.Path = strings.Trim(Launcher.s3folder.Path, "/")
}

func (t *launcher) Run() {
	errors := make(chan any)
	go t.downloadGame(errors)
	go t.downloadState(errors)
	for i := 0; i < 2; i++ {
		if err := <-errors; err != nil {
			panic(err)
		}
	}
	os.Chdir("factorio")
	Sitter.Run()
}

func (t *launcher) downloadGame(errors chan any) {
	defer func() { errors <- recover() }()
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
	fmt.Println("Downloaded game files")
}

func (t *launcher) downloadState(channel chan any) {
	defer func() { channel <- recover() }()
	// check if we need to do this
	_, err := os.Stat("factorio/saves")
	if err == nil {
		return // files are already copied over
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	// start downloaders
	queue := make(chan *string, 5)
	for i := 0; i < cap(queue); i++ {
		go func() {
			for key := range queue {
				if key == nil {
					return
				}
				t.downloadOneFile(key)
			}
		}()
	}
	// enumerate files from s3
	log.Printf("Downloading save files from %s\n", t.s3folder.String())
	request := &s3.ListObjectsInput{
		Bucket: aws.String(t.s3folder.Host),
		Prefix: aws.String(t.s3folder.Path + "/"),
	}
	for nextPage := aws.String("first"); nextPage != nil; nextPage = request.Marker {
		response, err := t.s3.ListObjects(request)
		if err != nil {
			panic(err)
		}
		for _, object := range response.Contents {
			// dunno if this can be nil, but if it ever happens there will be a deadlock
			if object.Key != nil {
				queue <- object.Key
			}
		}
		request.Marker = response.NextMarker
	}
	// wait for downloaders to finish
	for i := 0; i < cap(queue); i++ {
		queue <- nil
	}
	fmt.Println("Downloaded save and config/mod files")
}

func (t *launcher) downloadOneFile(key *string) {
	fmt.Printf("Downloading %s\n", *key)
	destPath := strings.TrimPrefix(*key, t.s3folder.Path+"/")
	// download the source file
	response, err := t.s3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(t.s3folder.Host),
		Key:    key,
	})
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()
	// create the destination file
	file, err := os.Create(destPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	// copy the file
	_, err = io.Copy(file, response.Body)
	if err != nil {
		panic(err)
	}
}

func (t *launcher) uploadSave() {
	// get the most recent save
	var mostRecent os.FileInfo
	var mostRecentTime time.Time
	filepath.Walk("saves", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".zip") {
			modTime := info.ModTime()
			if mostRecentTime.Before(modTime) {
				mostRecentTime = info.ModTime()
				mostRecent = info
			}
		}
		return nil
	})
	if mostRecent == nil {
		log.Println("No save files found")
		return
	}
	// upload the save
	log.Printf("Uploading save %s\n", mostRecent.Name())
	file, err := os.Open(mostRecent.Name())
	if err != nil {
		fmt.Printf("Error opening file: %s\n", err)
		return
	}
	defer file.Close()
	_, err = t.s3.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(Launcher.s3folder.Host),
		Key:    aws.String(Launcher.s3folder.Path + "/" + mostRecent.Name()),
		Body:   file,
	})
	if err != nil {
		fmt.Printf("Error uploading file: %s\n", err)
	}
}
